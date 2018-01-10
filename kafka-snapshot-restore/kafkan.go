package main

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func main() {
	env := ""
	brokers := []string{"kafka-mirror-3", "kafka-mirror-4", "kafka-mirror-5"}
	mountPoints := []string{"/dev/sds", "/dev/sdt", "/dev/sdu"}

	sess := session.Must(session.NewSession())
	svc := ec2.New(sess)

	getSnapshot(svc, env, brokers, mountPoints)
	// getNewKafkaVolumes(svc, env)

}

func getSnapshot(svc *ec2.EC2, environment string, brokers, mountPoints []string) {
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

	for _, snapshot := range result.Snapshots {
		for _, tag := range snapshot.Tags {
			if *tag.Key == *aws.String("Restore") && *tag.Value == *aws.String("true") {
				for _, definedTags := range snapshot.Tags {
					for _, broker := range brokers {
						if *definedTags.Key == *aws.String("Broker") && *definedTags.Value == *aws.String(broker) {
							for _, mountTag := range snapshot.Tags {
								for _, mount := range mountPoints {
									if *mountTag.Key == *aws.String("Mount") && *mountTag.Value == *aws.String(mount) {
										fmt.Println()
										fmt.Printf("%v: %v \n", *tag.Key, *tag.Value)
										fmt.Printf("%v: %v === %v \n", *definedTags.Key, *definedTags.Value, broker)
										fmt.Println(*mountTag.Key, *mountTag.Value)
										createVol(svc, *snapshot.SnapshotId, *definedTags.Value, *mountTag.Value, environment)
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

func createVol(svc *ec2.EC2, snapshotID, broker, mountTag, env string) (string, string) {
	volTagName := fmt.Sprintf("%v-%v-%v", broker, mountTag, snapshotID)

	input := &ec2.CreateVolumeInput{
		// need to find out how to dynamic AZ
		AvailabilityZone: aws.String("us-east-1a"),
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

	fmt.Println(*result.SnapshotId)
	fmt.Println(*result.VolumeId)
	return *result.VolumeId, mountTag

}

// Get new kafka instance volumes
func getNewKafkaVolumes(svc *ec2.EC2, environment string) {
	volResult, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{
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
		},
	})

	if err != nil {
		log.Fatal(err)
	}

	for _, reservation := range volResult.Reservations {
		for _, instance := range reservation.Instances {
			for _, blkdev := range instance.BlockDeviceMappings {
				describeVols(svc, *blkdev.Ebs.VolumeId)
			}
		}
	}
}

// filter out /dev/sda1
func describeVols(svc *ec2.EC2, volID string) {
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
				fmt.Printf("%v: %v\n", *attachment.VolumeId, *attachment.Device)
				// detachVols(svc, *attachment.VolumeId)
			}
		}
	}
}

// detach new kafka volumes
func detachVols(svc *ec2.EC2, volID string) {
	detachResult, err := svc.DetachVolume(&ec2.DetachVolumeInput{
		VolumeId: aws.String(volID),
	})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(detachResult)

}

// mount restored volumes to new kafka instances
func attachVolume(svc *ec2.EC2, VolID, mountTagID, environment string) {

	newInstanceID, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{
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
		},
	})

	if err != nil {
		log.Fatal(err)
	}

	for _, reservation := range newInstanceID.Reservations {
		for _, instance := range reservation.Instances {
			attachResult, err := svc.AttachVolume(&ec2.AttachVolumeInput{
				Device:     aws.String(mountTagID),
				InstanceId: aws.String(*instance.InstanceId),
				VolumeId:   aws.String(VolID),
			})

			if err != nil {
				log.Fatal(err)
			}

			fmt.Println(attachResult)
		}
	}

}
