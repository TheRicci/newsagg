package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/mmcdole/gofeed"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Article struct {
	ID          bson.ObjectID `bson:"_id,omitempty" json:"id"`
	Topic       string        `bson:"topic"         json:"topic"`
	Source      string        `bson:"source"        json:"source"`
	Title       string        `bson:"title"         json:"title"`
	URL         string        `bson:"url"           json:"url"`
	Summary     string        `bson:"summary"       json:"summary"`
	PublishedAt time.Time     `bson:"published_at"  json:"published_at"`
	FetchedAt   time.Time     `bson:"fetched_at"    json:"fetched_at"`
}

type FeedSource struct {
	Topic string
	Name  string
	URL   string
}

var sources = []FeedSource{
	{Topic: "ufology",      Name: "openminds",       URL: "https://openminds.tv/feed"},
	{Topic: "ufology",      Name: "newsnation",      URL: "https://newsnationnow.com/space/ufo/feed"},
	{Topic: "neuroscience", Name: "neurosciencenews", URL: "https://neurosciencenews.com/feed"},
	{Topic: "neuroscience", Name: "sciencedaily",    URL: "https://www.sciencedaily.com/rss/mind_brain/neuroscience.xml"},
	{Topic: "finance",      Name: "marketwatch",     URL: "https://feeds.marketwatch.com/marketwatch/topstories"},
	{Topic: "finance",      Name: "yahoofinance",    URL: "https://finance.yahoo.com/news/rssindex"},
}

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

	// Declare the queue — idempotent, safe to call every startup
	_, err = ch.QueueDeclare(
		"articles.new", // name
		true,            // durable — survives RabbitMQ restarts
		false,           // auto-delete
		false,           // exclusive
		false,           // no-wait
		nil,             // args
	)
	if err != nil {
		return nil, nil, fmt.Errorf("queue declare: %w", err)
	}

	log.Println("connected to rabbitmq, queue articles.new ready")
	return conn, ch, nil
}

func ensureIndex(col *mongo.Collection) error {
	idx := mongo.IndexModel{
		Keys:    bson.D{{Key: "url", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	_, err := col.Indexes().CreateOne(context.Background(), idx)
	return err
}

func publishArticle(ch *amqp.Channel, article Article) error {
	body, err := json.Marshal(article)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	return ch.Publish(
		"",              // exchange — empty means default direct exchange
		"articles.new",  // routing key — sends to queue with this name
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // survives RabbitMQ restarts
			Body:         body,
		},
	)
}

func fetchFeed(col *mongo.Collection, ch *amqp.Channel, src FeedSource) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(src.URL)
	if err != nil {
		log.Printf("error fetching %s: %v", src.Name, err)
		return
	}

	newCount := 0
	for _, item := range feed.Items {
		published := time.Now()
		if item.PublishedParsed != nil {
			published = *item.PublishedParsed
		}

		article := Article{
			Topic:       src.Topic,
			Source:      src.Name,
			Title:       item.Title,
			URL:         item.Link,
			Summary:     item.Description,
			PublishedAt: published,
			FetchedAt:   time.Now(),
		}

		result, err := col.InsertOne(context.Background(), article)
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				continue
			}
			log.Printf("insert error for %s: %v", src.Name, err)
			continue
		}

		// Set the ID from MongoDB so the event carries the full article reference
		article.ID = result.InsertedID.(bson.ObjectID)

		// Publish to RabbitMQ
		if err := publishArticle(ch, article); err != nil {
			log.Printf("publish error for %s: %v", src.Name, err)
		}
		newCount++
	}

	log.Printf("fetched %s (%s): %d new articles", src.Name, src.Topic, newCount)
}

func runFetchLoop(col *mongo.Collection, ch *amqp.Channel) {
	for {
		log.Println("starting fetch cycle")
		for _, src := range sources {
			fetchFeed(col, ch, src)
		}
		log.Println("fetch cycle complete, sleeping 30m")
		time.Sleep(30 * time.Minute)
	}
}

func main() {
	log.Println("fetcher starting up")

	client, err := connectMongo()
	if err != nil {
		log.Fatalf("failed to connect to mongodb: %v", err)
	}
	col := client.Database("newsagg").Collection("articles")

	if err := ensureIndex(col); err != nil {
		log.Fatalf("failed to create index: %v", err)
	}

	conn, ch, err := connectRabbitMQ()
	if err != nil {
		log.Fatalf("failed to connect to rabbitmq: %v", err)
	}
	defer conn.Close()
	defer ch.Close()

	go runFetchLoop(col, ch)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "fetcher ok")
	})

	log.Println("fetcher listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
