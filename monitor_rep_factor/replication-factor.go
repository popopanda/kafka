package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/bluele/slack"

	"gopkg.in/gomail.v2"
)

func checkKafkaRep() {
	zk := "zookeeper.service.consul"
	zkPort := "2181"
	kafkaCMD := fmt.Sprintf("/opt/kafka/kafka_2.10-0.10.2.0/bin/kafka-topics.sh --zookeeper %v:%v --describe | /bin/grep -E 'ReplicationFactor:1|ReplicationFactor:2'", zk, zkPort)
	kafkaCount := fmt.Sprintf("/opt/kafka/kafka_2.10-0.10.2.0/bin/kafka-topics.sh --zookeeper %v:%v --describe | /bin/grep -E 'ReplicationFactor:1|ReplicationFactor:2' | wc -l", zk, zkPort)
	cmd, _ := exec.Command("/bin/sh", "-c", kafkaCMD).Output()
	cmdCount, _ := exec.Command("/bin/sh", "-c", kafkaCount).Output()
	if len(cmd) > 0 {
		sendMail(string(cmd), string(cmdCount))
		sendSlack(string(cmd), string(cmdCount))
	} else {
		fmt.Println("We are good. All topics have a replication factor of 3.")
		os.Exit(0)
	}
}

func sendSlack(message, count string) {
	webHookURL := os.Getenv("WEBHOOKURL")
	webHookURLSlice := strings.Split(webHookURL, ",")
	env := os.Getenv("ENV")
	initText := fmt.Sprintf("[%v] %v Kafka Topics do not have a ReplicationFactor of 3", env, count)

	for _, i := range webHookURLSlice {
		hook := slack.NewWebHook(i)
		err := hook.PostMessage(&slack.WebHookPostPayload{
			Text: initText,
			Attachments: []*slack.Attachment{
				{Text: message, Color: "danger"},
			},
		})
		if err != nil {
			log.Fatal(err)
		}
	}
}

func sendMail(message, count string) {
	env := os.Getenv("ENV")
	toEmail := os.Getenv("TO_EMAIL")
	subject := fmt.Sprintf("[%v] %v Kafka Topics do not have a Replication Factor of 3", env, count)
	smtpServer := "smtp.gmail.com"
	smtpPort := 587
	smtpUsername := os.Getenv("MAIL_USER")
	smtpPassword := os.Getenv("MAIL_PASS")
	fromUserEmail := smtpUsername
	fromUserName := "GoKafka"

	msg := gomail.NewMessage()
	msg.SetHeaders(map[string][]string{
		"From":    {msg.FormatAddress(fromUserEmail, fromUserName)},
		"To":      {toEmail},
		"Subject": {subject},
	})

	msg.SetBody("text/plain", string(message))

	emailDelivery := gomail.NewDialer(smtpServer, smtpPort, smtpUsername, smtpPassword)

	if err := emailDelivery.DialAndSend(msg); err != nil {
		log.Fatal(err)
	}
}

func main() {
	checkKafkaRep()
}
