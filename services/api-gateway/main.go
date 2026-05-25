package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Article mirrors the fetcher's document structure
type Article struct {
	ID           bson.ObjectID `bson:"_id,omitempty"  json:"id"`
	Topic        string        `bson:"topic"          json:"topic"`
	Source       string        `bson:"source"         json:"source"`
	Title        string        `bson:"title"          json:"title"`
	URL          string        `bson:"url"            json:"url"`
	Summary      string        `bson:"summary"        json:"summary"`
	PublishedAt  time.Time     `bson:"published_at"   json:"published_at"`
	FetchedAt    time.Time     `bson:"fetched_at"     json:"fetched_at"`
	AISummary    string        `bson:"ai_summary"     json:"ai_summary,omitempty"`
	AIKeyPoints  []string      `bson:"ai_key_points"  json:"ai_key_points,omitempty"`
	AIContext    string        `bson:"ai_context"     json:"ai_context,omitempty"`
	AITags       []string      `bson:"ai_tags"        json:"ai_tags,omitempty"`
	AIConfidence string        `bson:"ai_confidence"  json:"ai_confidence,omitempty"`
	AIEnrichedAt *time.Time    `bson:"ai_enriched_at" json:"ai_enriched_at,omitempty"`
}

// TopicCount is returned by the /topics endpoint
type TopicCount struct {
	Topic string `bson:"_id"   json:"topic"`
	Count int    `bson:"count" json:"count"`
}

var col *mongo.Collection

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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// GET /health
func handleHealth(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "api-gateway ok")
}

// GET /topics
func handleTopics(w http.ResponseWriter, r *http.Request) {
	pipeline := bson.A{
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$topic"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
	}

	cursor, err := col.Aggregate(context.Background(), pipeline)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	var results []TopicCount
	if err := cursor.All(context.Background(), &results); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// GET /articles?topic=x&limit=20&page=1
func handleArticles(w http.ResponseWriter, r *http.Request) {
	filter := bson.D{}
	if topic := r.URL.Query().Get("topic"); topic != "" {
		filter = bson.D{{Key: "topic", Value: topic}}
	}

	limit := int64(20)
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.ParseInt(l, 10, 64); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	page := int64(1)
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.ParseInt(p, 10, 64); err == nil && parsed > 0 {
			page = parsed
		}
	}
	skip := (page - 1) * limit

	opts := options.Find().
		SetSort(bson.D{{Key: "published_at", Value: -1}}).
		SetLimit(limit).
		SetSkip(skip)

	cursor, err := col.Find(context.Background(), filter, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	var articles []Article
	if err := cursor.All(context.Background(), &articles); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"page":     page,
		"limit":    limit,
		"articles": articles,
	})
}

// GET /articles/{id}
func handleArticleByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := bson.ObjectIDFromHex(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var article Article
	err = col.FindOne(context.Background(), bson.D{{Key: "_id", Value: id}}).Decode(&article)
	if err == mongo.ErrNoDocuments {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, article)
}

func main() {
	log.Println("api-gateway starting up")

	client, err := connectMongo()
	if err != nil {
		log.Fatalf("failed to connect to mongodb: %v", err)
	}
	col = client.Database("newsagg").Collection("articles")

	r := chi.NewRouter()
	r.Use(middleware.Logger)    // logs every request
	r.Use(middleware.Recoverer) // catches panics, returns 500

	r.Get("/health", handleHealth)
	r.Get("/topics", handleTopics)
	r.Get("/articles", handleArticles)
	r.Get("/articles/{id}", handleArticleByID)

	log.Println("api-gateway listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", r))
}
