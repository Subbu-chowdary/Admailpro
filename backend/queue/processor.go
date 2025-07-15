package queue

import (
	"context"
	"email-sender/backend/config"
	"email-sender/backend/db"
	"email-sender/backend/health"
	"email-sender/backend/models"
	"email-sender/backend/smtp"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/hibiken/asynq"
)

var roundRobinIndex int32

func StartWorker() {
	cfg := config.GetConfig()
	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{Concurrency: 10},
	)
	mux := asynq.NewServeMux()
	mux.HandleFunc("email:send", processEmailTask)
	if err := srv.Run(mux); err != nil {
		log.Fatalf("Could not run worker: %v", err)
	}
}

func processEmailTask(ctx context.Context, t *asynq.Task) error {
	var job models.EmailJob
	if err := json.Unmarshal(t.Payload(), &job); err != nil {
		return err
	}
	cfg := config.GetConfig()
	sesConfigs := cfg.SESConfigs
	if len(sesConfigs) == 0 {
		return fmt.Errorf("no SES configurations available")
	}
	index := int(atomic.AddInt32(&roundRobinIndex, 1) - 1)
	selectedConfig := sesConfigs[index%len(sesConfigs)]
	hm := health.NewHealthManager()
	healthiest := hm.GetHealthiestPair()
	if healthiest.Health < 50 {
		index = (index + 1) % len(sesConfigs)
		selectedConfig = sesConfigs[index%len(sesConfigs)]
	}
	job.Subdomain = selectedConfig.Subdomain
	job.IP = selectedConfig.IP
	hm.UpdateHealth(selectedConfig.Subdomain, selectedConfig.IP, 0.1, 0.05, 0.02)
	if err := smtp.SendEmail(job); err != nil {
		return err
	}
	if err := db.GetCollection("email_jobs").FindOneAndUpdate(
		context.Background(),
		map[string]string{"_id": job.ID},
		map[string]interface{}{"$set": map[string]string{"status": "sent"}},
	).Err(); err != nil {
		return err
	}
	return nil
}