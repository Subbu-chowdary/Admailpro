package api

import (
	"email-sender/backend/cloaker"
	"email-sender/backend/config"
	"email-sender/backend/db"
	"email-sender/backend/models"
	"email-sender/backend/utils"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/valyala/fasthttp"
)

func UploadCSVHandler(ctx *fasthttp.RequestCtx) {
	// Check content type
	contentType := string(ctx.Request.Header.ContentType())
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		ctx.Error("Invalid content type", fasthttp.StatusBadRequest)
		return
	}

	form, err := ctx.MultipartForm()
	if err != nil {
		ctx.Error("Failed to parse multipart form", fasthttp.StatusBadRequest)
		return
	}

	files := form.File["file"]
	if len(files) == 0 {
		ctx.Error("No file provided", fasthttp.StatusBadRequest)
		return
	}

	file := files[0]
	src, err := file.Open()
	if err != nil {
		ctx.Error("Failed to open uploaded file", fasthttp.StatusInternalServerError)
		return
	}
	defer src.Close()

	reader := csv.NewReader(src)
	records, err := reader.ReadAll()
	if err != nil {
		ctx.Error("Invalid CSV format", fasthttp.StatusBadRequest)
		return
	}

	client := asynq.NewClient(asynq.RedisClientOpt{Addr: config.GetConfig().RedisAddr})
	defer client.Close()

	success := 0
	for i, record := range records {
		if i == 0 || len(record) < 3 {
			continue
		}

		req := models.EmailRequest{
			Recipient: strings.TrimSpace(record[0]),
			Subject:   strings.TrimSpace(record[1]),
			HTML:      strings.TrimSpace(record[2]),
		}

		cloakedHTML, _, err := cloaker.CloakLinks(req.HTML)
		if err != nil {
			log.Printf("Failed to cloak links (row %d): %v", i+1, err)
			continue
		}
		req.HTML = cloakedHTML

		job := models.EmailJob{
			ID:        utils.GenerateID(),
			Request:   req,
			Status:    "queued",
			Subdomain: "", IP: "",
		}
		if email, ok := ctx.UserValue("email").(string); ok {
			job.UserID = email
		}

		if err := db.SaveEmailJob(&job); err != nil {
			log.Printf("Failed to save job (row %d): %v", i+1, err)
			continue
		}

		payload, _ := json.Marshal(job)
		task := asynq.NewTask("email:send", payload, asynq.MaxRetry(3))
		if _, err := client.Enqueue(task); err != nil {
			log.Printf("Failed to enqueue (row %d): %v", i+1, err)
			continue
		}

		success++
	}

	ctx.SetStatusCode(fasthttp.StatusAccepted)
	ctx.SetBody([]byte(fmt.Sprintf("Processed %d of %d email jobs", success, len(records)-1)))
}
