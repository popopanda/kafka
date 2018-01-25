package main

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func main() {
	// Set Environment
	env := "uat2"
	// Set Kafka-mirror brokers to ID the snapshots to restore
	brokers := map[string]string{
		"kafka-mirror-0": "us-east-1b",
		"kafka-mirror-1": "us-east-1d",
		"kafka-mirror-2": "us-east-1e",
	}
	// Specify the mount points for attachment
	mountPoints := []string{"/dev/sds", "/dev/sdt", "/dev/sdu"}

	// Create session
	sess := session.Must(session.NewSession())
	svc := ec2.New(sess)

	// Detach vols
	getNewKafkaVolumes(svc, env)
	fmt.Println("Waiting for Volumes to finish detaching...")
	time.Sleep(time.Second * 300)

	// Reboot instances
	fmt.Println("Rebooting...")
	instanceReboot(svc, env)
	time.Sleep(time.Second * 300)

	// Create Volumes and attach
	fmt.Println("Creating Volumes")
	getSnapshot(svc, env, brokers, mountPoints)
	fmt.Println("Waiting for new volumes to finish attaching")
	time.Sleep(time.Second * 300)

	// Add logic to repopulate zookeeper?

	// reboot instances
	fmt.Println("Rebooting instances now")
	instanceReboot(svc, env)
	fmt.Println("Waiting...")
	time.Sleep(time.Second * 300)
	fmt.Println("Kafka Restored")
}

func getSnapshot(svc *ec2.EC2, environment string, brokers map[string]string, mountPoints []string) {
	// Locate the Kafka snapshots
	result, err := svc.DescribeSnapshots(&ec2.DescribeSnapshotsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("tag:environment"),
				Values: []*string{
					aws.String(environment),
				},
			},
		},
	})

	if err != nil {
		log.Fatal(err)
	}
	//Find the correct snapshots and attach the volumes
	for _, snapshot := range result.Snapshots {
		for _, tag := range snapshot.Tags {
			// Find tag of Restore == true
			if *tag.Key == *aws.String("Restore") && *tag.Value == *aws.String("true") {
				for _, definedTags := range snapshot.Tags {
					for broker, az := range brokers {
						// Find Broker tag and match it to the brokers map variable
						if *definedTags.Key == *aws.String("Broker") && *definedTags.Value == *aws.String(broker) {
							for _, mountTag := range snapshot.Tags {
								for _, mount := range mountPoints {
									// Find the Mount tag and match it to the mountPoints slices
									if *mountTag.Key == *aws.String("Mount") && *mountTag.Value == *aws.String(mount) {
										// Create the volume
										createVolID, createVolAZ := createVol(svc, *snapshot.SnapshotId, *definedTags.Value, *mountTag.Value, environment, az)
										fmt.Printf("Waiting for %v to finish creating...\n", createVolID)
										// Sleep for 10 seconds
										time.Sleep(10 * time.Second)
										// Attach the volume to the host
										attachVolume(svc, createVolID, *mountTag.Value, environment, createVolAZ, *definedTags.Value)
									}
								}
							}
						}
					}
				}
			} else {
				continue
			}
		}
	}
}

// Create the volumes
func createVol(svc *ec2.EC2, snapshotID, broker, mountTag, env, az string) (string, string) {
	volTagName := fmt.Sprintf("%v-%v-%v", broker, mountTag, snapshotID)

	input := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(az),
		SnapshotId:       aws.String(snapshotID),
		VolumeType:       aws.String("gp2"),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String("volume"),
				Tags: []*ec2.Tag{
					{
						Key:   aws.String("Name"),
						Value: aws.String(volTagName),
					},
					{
						Key:   aws.String("Broker"),
						Value: aws.String(broker),
					},
					{
						Key:   aws.String("SnapshotID"),
						Value: aws.String(snapshotID),
					},
					{
						Key:   aws.String("Mount"),
						Value: aws.String(mountTag),
					},
					{
						Key:   aws.String("Restore"),
						Value: aws.String("true"),
					},
					{
						Key:   aws.String("Environment"),
						Value: aws.String(env),
					},
				},
			},
		},
	}

	result, err := svc.CreateVolume(input)
	if err != nil {
		log.Fatal(err)
	}

	return *result.VolumeId, *result.AvailabilityZone

}

// Get new kafka instance volumes
func getNewKafkaVolumes(svc *ec2.EC2, environment string) {
	volResult, err := describeInstances(svc, environment)

	if err != nil {
		log.Fatal(err)
	}

	for _, reservation := range volResult.Reservations {
		for _, instance := range reservation.Instances {
			for _, blkdev := range instance.BlockDeviceMappings {
				filterNewKafkaRootVol(svc, *blkdev.Ebs.VolumeId)
			}
		}
	}
}

// filter out /dev/sda1
func filterNewKafkaRootVol(svc *ec2.EC2, volID string) {
	input := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("volume-id"),
				Values: []*string{
					aws.String(volID),
				},
			},
		},
	}
	result, err := svc.DescribeVolumes(input)

	if err != nil {
		log.Fatal(err)
	}

	for _, volume := range result.Volumes {
		for _, attachment := range volume.Attachments {
			if *attachment.Device == "/dev/sda1" {
				continue
			} else {
				detachVols(svc, *attachment.VolumeId)
			}
		}
	}
}

// detach new kafka volumes
func detachVols(svc *ec2.EC2, volID string) {
	detachResult, err := svc.DetachVolume(&ec2.DetachVolumeInput{
		Force:    aws.Bool(true),
		VolumeId: aws.String(volID),
	})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Volume: %v: %v\n", volID, *detachResult.State)
}

// mount restored volumes to new kafka instances
func attachVolume(svc *ec2.EC2, VolID, mountTagID, environment, az, broker string) {

	newInstanceID, err := describeInstances(svc, environment)

	if err != nil {
		log.Fatal(err)
	}

	for _, reservation := range newInstanceID.Reservations {
		for _, instance := range reservation.Instances {
			if *instance.Placement.AvailabilityZone == az {
				attachResult, err := svc.AttachVolume(&ec2.AttachVolumeInput{
					Device:     aws.String(mountTagID),
					InstanceId: aws.String(*instance.InstanceId),
					VolumeId:   aws.String(VolID),
				})
				fmt.Println(*attachResult.State)

				if err != nil {
					log.Fatal(err)
				}
			} else {
				continue
			}

			if err != nil {
				log.Fatal(err)
			}

		}
	}
}

// Reboot instances
func instanceReboot(svc *ec2.EC2, environment string) *ec2.RebootInstancesOutput {

	newInstanceID, err := describeInstances(svc, environment)

	hosts := []string{}

	for _, reservation := range newInstanceID.Reservations {
		for _, instance := range reservation.Instances {
			hosts = append(hosts, *instance.InstanceId)
		}
	}

	rebootInput := &ec2.RebootInstancesInput{
		InstanceIds: aws.StringSlice(hosts),
	}

	rebootResult, err := svc.RebootInstances(rebootInput)

	if err != nil {
		log.Fatal(err)
	}

	return rebootResult
}

// Function to locate correct instances
func describeInstances(svc *ec2.EC2, environment string) (ec2.DescribeInstancesOutput, error) {
	instanceID, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("tag:Environment"),
				Values: []*string{
					aws.String(environment),
				},
			},
			{
				Name: aws.String("tag:Role"),
				Values: []*string{
					aws.String("kafka"),
				},
			},
			{
				Name: aws.String("instance-state-name"),
				Values: []*string{
					aws.String("running"),
				},
			},
		},
	})

	return *instanceID, err
}
