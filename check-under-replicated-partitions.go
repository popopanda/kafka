package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func main() {
	fmt.Println("Checking for kafka underreplicated partitions")

}

func sshProxy(username, bastion, port string) {

	sock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatal(err)
	}

	agent := agent.NewClient(sock)

	signers, err := agent.Signers()
	if err != nil {
		log.Fatal(err)
	}

	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signers...),
		},
	}

	sshConnect(bastion, port, sshConfig)

}

func sshConnect(hostname, port string, config *ssh.ClientConfig) {
	client, err := ssh.Dial("tcp", hostname+port, config)
	if err != nil {
		log.Fatal(err)
	}

	session, err := client.NewSession()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("We have a connection")
}
