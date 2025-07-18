// backend/main.go
package main

import (
	"log"
	"strings"

	"github.com/valyala/fasthttp"

	"email-sender/backend/api"
	"email-sender/backend/metrics"
	"email-sender/backend/queue"
	"email-sender/backend/redirect"
)

func main() {
	// Start background queue worker
	go queue.StartWorker()

	// Main request router
	requestHandler := func(ctx *fasthttp.RequestCtx) {
		switch string(ctx.Path()) {
		case "/api/signup":
			api.SignupHandler(ctx)
		case "/api/login":
			api.LoginHandler(ctx)
		case "/api/send-email": // For sending a single, ad-hoc email
			api.AuthMiddleware(api.SendEmailHandler)(ctx)
		case "/api/upload-csv": // Uploads only recipient emails, returns list_id
			api.AuthMiddleware(api.UploadCSVHandler)(ctx)
		case "/api/campaigns": // Creates campaign content (subject/html), returns campaign_id
			if ctx.IsPost() {
				api.AuthMiddleware(api.CreateCampaignHandler)(ctx)
			} else {
				ctx.Error("Method Not Allowed", fasthttp.StatusMethodNotAllowed)
			}
		case "/api/send-campaign": // Initiates sending a campaign to a recipient list
			if ctx.IsPost() {
				api.AuthMiddleware(api.SendCampaignHandler)(ctx)
			} else {
				ctx.Error("Method Not Allowed", fasthttp.StatusMethodNotAllowed)
			}
		case "/api/subdomains":
			if ctx.IsGet() {
				api.GetSubdomains(ctx)
			} else if ctx.IsPost() {
				api.AddSubdomain(ctx)
			}
		case "/metrics":
			metrics.Handler(ctx)
		case "/api/redirect":
			redirect.Handler(ctx)
		default:
			path := string(ctx.Path())
			if strings.HasPrefix(path, "/api/subdomains/") {
				if ctx.IsPut() {
					api.UpdateSubdomain(ctx)
				} else if ctx.IsDelete() {
					api.DeleteSubdomain(ctx)
				} else {
					ctx.Error("Method Not Allowed", fasthttp.StatusMethodNotAllowed)
				}
			} else {
				ctx.Error("Not Found", fasthttp.StatusNotFound)
			}
		}
	}

	// CORS Middleware
	corsHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
		ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
		ctx.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if string(ctx.Method()) == "OPTIONS" {
			ctx.SetStatusCode(fasthttp.StatusOK)
			return
		}
		requestHandler(ctx)
	}

	log.Println("🚀 Server running on http://localhost:8080")
	log.Fatal(fasthttp.ListenAndServe(":8080", corsHandler))
}