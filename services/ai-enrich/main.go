package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Article struct {
	ID      bson.ObjectID `bson:"_id,omitempty" json:"id"`
	Topic   string        `bson:"topic"         json:"topic"`
	Source  string        `bson:"source"        json:"source"`
	Title   string        `bson:"title"         json:"title"`
	URL     string        `bson:"url"           json:"url"`
	Summary string        `bson:"summary"       json:"summary"`
}

type EnrichmentResult struct {
	Summary    string   `json:"summary"`
	KeyPoints  []string `json:"key_points"`
	Context    string   `json:"context"`
	Tags       []string `json:"tags"`
	Confidence string   `json:"confidence"`
}

type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

type BatchResult struct {
	Index      int              `json:"index"`
	Enrichment EnrichmentResult `json:"enrichment"`
}

var (
	col        *mongo.Collection
	geminiKey  string
	httpClient = &http.Client{Timeout: 45 * time.Second}
)

const (
	batchSize    = 10
	batchTimeout = 30 * time.Second
	geminiModel  = "gemini-2.5-flash"
	rpmLimit     = 10
	maxRetries   = 3
)

func connectMongo() (*mongo.Client, error) {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}
	log.Printf("connecting to mongodb at %s", uri)
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("mongo ping: %w", err)
	}
	log.Println("connected to mongodb")
	return client, nil
}

func connectRabbitMQ() (*amqp.Connection, *amqp.Channel, error) {
	uri := os.Getenv("RABBITMQ_URI")
	if uri == "" {
		uri = "amqp://guest:guest@localhost:5672/"
	}
	log.Printf("connecting to rabbitmq at %s", uri)
	conn, err := amqp.Dial(uri)
	if err != nil {
		return nil, nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	if err := ch.Qos(batchSize, 0, false); err != nil {
		return nil, nil, fmt.Errorf("qos: %w", err)
	}

	// 1. dead letter exchange
	if err := ch.ExchangeDeclare(
		"articles.dlx", "direct", true, false, false, false, nil,
	); err != nil {
		return nil, nil, fmt.Errorf("dlx declare: %w", err)
	}

	// 2. retry queue — 60s TTL, routes back to articles.new on expiry
	if _, err := ch.QueueDeclare(
		"articles.retry", true, false, false, false,
		amqp.Table{
			"x-dead-letter-exchange":    "",
			"x-dead-letter-routing-key": "articles.new",
			"x-message-ttl":             int32(60000),
		},
	); err != nil {
		return nil, nil, fmt.Errorf("retry queue declare: %w", err)
	}
	if err := ch.QueueBind("articles.retry", "articles.retry", "articles.dlx", false, nil); err != nil {
		return nil, nil, fmt.Errorf("retry queue bind: %w", err)
	}

	// 3. failed queue — permanent DLQ
	if _, err := ch.QueueDeclare(
		"articles.failed", true, false, false, false, nil,
	); err != nil {
		return nil, nil, fmt.Errorf("failed queue declare: %w", err)
	}
	if err := ch.QueueBind("articles.failed", "articles.failed", "articles.dlx", false, nil); err != nil {
		return nil, nil, fmt.Errorf("failed queue bind: %w", err)
	}

	// 4. main queue — routes failures to DLX → retry
	if _, err := ch.QueueDeclare(
		"articles.new", true, false, false, false,
		amqp.Table{
			"x-dead-letter-exchange":    "articles.dlx",
			"x-dead-letter-routing-key": "articles.retry",
		},
	); err != nil {
		return nil, nil, fmt.Errorf("queue declare: %w", err)
	}

	log.Println("connected to rabbitmq, DLQ topology ready")
	return conn, ch, nil
}

// retryCount reads the x-death header to count how many times a message has failed
func retryCount(msg amqp.Delivery) int {
	deaths, ok := msg.Headers["x-death"]
	if !ok {
		return 0
	}
	deathList, ok := deaths.([]interface{})
	if !ok {
		return 0
	}
	total := 0
	for _, d := range deathList {
		if table, ok := d.(amqp.Table); ok {
			if count, ok := table["count"]; ok {
				switch v := count.(type) {
				case int64:
					total += int(v)
				case int32:
					total += int(v)
				}
			}
		}
	}
	return total
}

func enrichBatch(articles []Article) (map[int]EnrichmentResult, error) {
	var prompt bytes.Buffer
	prompt.WriteString("You are a news enrichment assistant.\n")
	prompt.WriteString("For each article below, fetch the URL and read the full content if accessible.\n")
	prompt.WriteString("Return as much useful enrichment as you can find.\n\n")
	prompt.WriteString("Return ONLY a JSON array, no markdown, no explanation.\n")
	prompt.WriteString("Format:\n")
	prompt.WriteString(`[{"index": 0, "enrichment": {"summary": "one sentence", "key_points": ["...", "..."], "context": "background paragraph or empty string", "tags": ["tag1", "tag2"], "confidence": "high|medium|low"}}]`)
	prompt.WriteString("\n\n")
	prompt.WriteString("confidence levels:\n")
	prompt.WriteString("- high: accessed full article\n")
	prompt.WriteString("- medium: accessed partial content\n")
	prompt.WriteString("- low: working only from title and description\n\n")

	for i, a := range articles {
		fmt.Fprintf(&prompt, "Article %d:\nTitle: %s\nDescription: %s\nURL: %s\n\n",
			i, a.Title, a.Summary, a.URL)
	}

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{Parts: []GeminiPart{{Text: prompt.String()}}},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		geminiModel, geminiKey,
	)

	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini error %d: %s", resp.StatusCode, string(b))
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	text := geminiResp.Candidates[0].Content.Parts[0].Text
	text = string(bytes.TrimSpace([]byte(text)))
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		text = strings.Join(lines[1:len(lines)-1], "\n")
	}

	var results []BatchResult
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		return nil, fmt.Errorf("parse enrichments: %w (raw: %s)", err, text)
	}

	enrichments := make(map[int]EnrichmentResult)
	for _, r := range results {
		enrichments[r.Index] = r.Enrichment
	}
	return enrichments, nil
}

func saveEnrichments(articles []Article, enrichments map[int]EnrichmentResult) {
	for i, article := range articles {
		enrichment, ok := enrichments[i]
		if !ok {
			continue
		}
		_, err := col.UpdateOne(
			context.Background(),
			bson.D{{Key: "_id", Value: article.ID}},
			bson.D{{Key: "$set", Value: bson.D{
				{Key: "ai_summary", Value: enrichment.Summary},
				{Key: "ai_key_points", Value: enrichment.KeyPoints},
				{Key: "ai_context", Value: enrichment.Context},
				{Key: "ai_tags", Value: enrichment.Tags},
				{Key: "ai_confidence", Value: enrichment.Confidence},
				{Key: "ai_enriched_at", Value: time.Now()},
			}}},
		)
		if err != nil {
			log.Printf("failed to save enrichment for %s: %v", article.ID, err)
		}
	}
}

// collectBatch collects up to batchSize articles from the queue
// messages that have failed maxRetries times are sent to articles.failed
func collectBatch(ch *amqp.Channel, deliveries <-chan amqp.Delivery) ([]Article, []amqp.Delivery) {
	var articles []Article
	var msgs []amqp.Delivery
	deadline := time.After(batchTimeout)

	for len(articles) < batchSize {
		select {
		case msg, ok := <-deliveries:
			if !ok {
				return articles, msgs
			}

			// check retry count — send to failed queue if exceeded
			count := retryCount(msg)
			if count >= maxRetries {
				log.Printf("message exceeded max retries (%d), sending to articles.failed", count)
				ch.Publish("articles.dlx", "articles.failed", false, false,
					amqp.Publishing{
						ContentType:  "application/json",
						DeliveryMode: amqp.Persistent,
						Body:         msg.Body,
					},
				)
				msg.Ack(false) // ack original so it leaves articles.new
				continue
			}

			var article Article
			if err := json.Unmarshal(msg.Body, &article); err != nil {
				log.Printf("failed to unmarshal message: %v", err)
				msg.Nack(false, false)
				continue
			}
			articles = append(articles, article)
			msgs = append(msgs, msg)

		case <-deadline:
			return articles, msgs
		}
	}
	return articles, msgs
}

func runEnrichLoop(ch *amqp.Channel) {
	deliveries, err := ch.Consume(
		"articles.new",
		"ai-enrich",
		false,
		false, false, false, nil,
	)
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	ticker := time.NewTicker(time.Minute / time.Duration(rpmLimit))
	defer ticker.Stop()

	backoff := 30 * time.Second
	maxBackoff := 10 * time.Minute

	log.Printf("ai-enrich loop started — batch size %d, timeout %s, RPM limit %d, max retries %d",
		batchSize, batchTimeout, rpmLimit, maxRetries)

	for {
		articles, msgs := collectBatch(ch, deliveries)
		if len(articles) == 0 {
			continue
		}

		log.Printf("enriching batch of %d articles", len(articles))

		<-ticker.C

		enrichments, err := enrichBatch(articles)
		if err != nil {
			log.Printf("gemini error: %v — retrying in %s", err, backoff)
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			for _, msg := range msgs {
				msg.Nack(false, false) // routes to DLX → articles.retry
			}
			continue
		}

		backoff = 30 * time.Second
		saveEnrichments(articles, enrichments)

		for _, msg := range msgs {
			msg.Ack(false)
		}

		log.Printf("enriched and saved %d articles", len(enrichments))
	}
}

func main() {
	log.Println("ai-enrich starting up")

	geminiKey = os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}

	client, err := connectMongo()
	if err != nil {
		log.Fatalf("failed to connect to mongodb: %v", err)
	}
	col = client.Database("newsagg").Collection("articles")

	conn, ch, err := connectRabbitMQ()
	if err != nil {
		log.Fatalf("failed to connect to rabbitmq: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ai-enrich ok")
	})
	go func() {
		log.Println("ai-enrich listening on :8082")
		log.Fatal(http.ListenAndServe(":8082", nil))
	}()

	runEnrichLoop(ch)
}
