package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

// SeedCategories contains the initial notification categories
var SeedCategories = []models.NotificationCategory{
	{
		ID:           "events",
		Name:         "Eventos da Cidade",
		Description:  "Receba notificações sobre eventos culturais, esportivos e comunitários acontecendo na cidade",
		DefaultOptIn: true,
		Active:       true,
		Order:        1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	},
	{
		ID:           "services",
		Name:         "Serviços Públicos",
		Description:  "Atualizações sobre serviços públicos, manutenções programadas e novos serviços disponíveis",
		DefaultOptIn: true,
		Active:       true,
		Order:        2,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	},
	{
		ID:           "alerts",
		Name:         "Alertas Importantes",
		Description:  "Alertas urgentes sobre segurança, clima, emergências e informações críticas",
		DefaultOptIn: true,
		Active:       true,
		Order:        3,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	},
	{
		ID:           "mei_opportunities",
		Name:         "Oportunidades MEI",
		Description:  "Vagas de trabalho, editais e oportunidades de negócio para microempreendedores",
		DefaultOptIn: false,
		Active:       true,
		Order:        4,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	},
	{
		ID:           "courses",
		Name:         "Cursos e Capacitação",
		Description:  "Cursos gratuitos, workshops e programas de capacitação profissional oferecidos pela prefeitura",
		DefaultOptIn: false,
		Active:       true,
		Order:        5,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	},
	{
		ID:           "health",
		Name:         "Saúde",
		Description:  "Campanhas de vacinação, programas de saúde preventiva e informações sobre unidades de saúde",
		DefaultOptIn: true,
		Active:       true,
		Order:        6,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	},
}

func main() {
	fmt.Println("🌱 Seeding notification categories...")

	// Load configuration
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize MongoDB
	config.InitMongoDB()
	if config.MongoDB == nil {
		log.Fatal("Failed to initialize MongoDB")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)

	// Check if categories already exist
	count, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to count existing categories: %v", err)
	}

	if count > 0 {
		fmt.Printf("⚠️  Found %d existing categories. Do you want to replace them? (y/N): ", count)
		var response string
		_, err := fmt.Scanln(&response)
		if err != nil {
			fmt.Println("❌ Error reading input")
			return
		}
		if response != "y" && response != "Y" {
			fmt.Println("❌ Seeding cancelled")
			return
		}

		// Delete existing categories
		result, err := collection.DeleteMany(ctx, bson.M{})
		if err != nil {
			log.Fatalf("Failed to delete existing categories: %v", err)
		}
		fmt.Printf("🗑️  Deleted %d existing categories\n", result.DeletedCount)
	}

	// Insert seed categories
	docs := make([]interface{}, len(SeedCategories))
	for i, cat := range SeedCategories {
		docs[i] = cat
	}

	result, err := collection.InsertMany(ctx, docs)
	if err != nil {
		log.Fatalf("Failed to insert categories: %v", err)
	}

	fmt.Printf("✅ Successfully seeded %d notification categories:\n", len(result.InsertedIDs))
	for _, cat := range SeedCategories {
		status := "✓"
		if !cat.Active {
			status = "✗"
		}
		defaultStr := ""
		if cat.DefaultOptIn {
			defaultStr = "(default: ON)"
		} else {
			defaultStr = "(default: OFF)"
		}
		fmt.Printf("  %s [%s] %s - %s %s\n", status, cat.ID, cat.Name, cat.Description, defaultStr)
	}

	fmt.Println("\n🎉 Seeding completed successfully!")
}
