package main

import (
	"fmt"
	"net/http"
	"time"
)

const (
	bucketName = "reviv-postgres-backup-20260817"
	backupFile = "../postgres-backup/backup.sql"
)

func main() {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		resp, err := client.Get("http://localhost:8080/health")
		if err != nil {
			fmt.Printf("health check failed: %v\n", err)
			return
		}

		if resp.StatusCode == http.StatusOK {
			fmt.Println("Server is reachable!")

			s3Key := fmt.Sprintf("backup-%d.sql", time.Now().Unix())

			err := uploadFile(bucketName, backupFile, s3Key)
			if err != nil {
				fmt.Printf("s=S3 upload failed: %v\n", err)
			}

		}
		resp.Body.Close()

		<-ticker.C
	}

}
