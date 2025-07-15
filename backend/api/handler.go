package api

import (
	"email-sender/backend/cloaker"
	"email-sender/backend/config"
	"email-sender/backend/db"
	"email-sender/backend/models"
	"email-sender/backend/utils"
	"encoding/json"

	"github.com/hibiken/asynq"
	"github.com/valyala/fasthttp"
)

func SendEmailHandler(ctx *fasthttp.RequestCtx) {
	var req models.EmailRequest
	if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
		ctx.Error("Invalid request", fasthttp.StatusBadRequest)
		return
	}

	cloakedHTML, _, err := cloaker.CloakLinks(req.HTML)
	if err != nil {
		ctx.Error("Failed to cloak links", fasthttp.StatusInternalServerError)
		return
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
		ctx.Error("Failed to save job", fasthttp.StatusInternalServerError)
		return
	}
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: config.GetConfig().RedisAddr})
	defer client.Close()
	payload, _ := json.Marshal(job)
	task := asynq.NewTask("email:send", payload, asynq.MaxRetry(3))
	if _, err := client.Enqueue(task); err != nil {
		ctx.Error("Failed to enqueue email", fasthttp.StatusInternalServerError)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

func GetSubdomains(ctx *fasthttp.RequestCtx) {
	pairs, err := db.GetIPPairs()
	if err != nil {
		ctx.Error("Failed to fetch subdomains", fasthttp.StatusInternalServerError)
		return
	}
	ctx.SetContentType("application/json")
	json.NewEncoder(ctx).Encode(pairs)
}

func AddSubdomain(ctx *fasthttp.RequestCtx) {
	var pair models.IPPair
	if err := json.Unmarshal(ctx.PostBody(), &pair); err != nil {
		ctx.Error("Invalid request", fasthttp.StatusBadRequest)
		return
	}
	if err := db.SaveIPPair(&pair); err != nil {
		ctx.Error("Failed to add subdomain", fasthttp.StatusInternalServerError)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusCreated)
}

func UpdateSubdomain(ctx *fasthttp.RequestCtx) {
	subdomain := ctx.UserValue("subdomain").(string)
	ip := ctx.UserValue("ip").(string)
	var pair models.IPPair
	if err := json.Unmarshal(ctx.PostBody(), &pair); err != nil {
		ctx.Error("Invalid request", fasthttp.StatusBadRequest)
		return
	}
	if err := db.UpdateIPPair(subdomain, ip, &pair); err != nil {
		ctx.Error("Failed to update subdomain", fasthttp.StatusInternalServerError)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusOK)
}

func DeleteSubdomain(ctx *fasthttp.RequestCtx) {
	subdomain := ctx.UserValue("subdomain").(string)
	ip := ctx.UserValue("ip").(string)
	if err := db.DeleteIPPair(subdomain, ip); err != nil {
		ctx.Error("Failed to delete subdomain", fasthttp.StatusInternalServerError)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}
