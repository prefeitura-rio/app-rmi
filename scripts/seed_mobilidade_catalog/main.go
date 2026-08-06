// Seed Mobilidade vehicle brands/models from CSV (+ stable "Outro" sentinel).
//
// Usage:
//
//	go run ./scripts/seed_mobilidade_catalog [path/to/catalog.csv]
//
// Default CSV: scripts/data/mobilidade-vehicle-catalog.sample.csv
//
// Rules (Apêndice B):
//   - UTF-8, comma-separated, header: marca,modelo,tipo
//   - tipo ∈ bicicleta_eletrica | autopropelido | ciclomotor
//   - "Outro" is NOT in the CSV; always upserted as brand_outro / model_outro with is_other=true
//   - Brand/model IDs are stable slugs (brand_<slug>, model_<brandslug>_<modelslug>)
package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	csvPath := "scripts/data/mobilidade-vehicle-catalog.sample.csv"
	if len(os.Args) > 1 {
		csvPath = os.Args[1]
	}

	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	config.InitMongoDB()
	if config.MongoDB == nil {
		log.Fatal("Failed to initialize MongoDB")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	rows, err := readCatalogCSV(csvPath)
	if err != nil {
		log.Fatalf("read csv: %v", err)
	}

	brands := config.MongoDB.Collection(config.AppConfig.MobilidadeBrandCollection)
	modelsCol := config.MongoDB.Collection(config.AppConfig.MobilidadeModelCollection)
	upsertOpts := options.Update().SetUpsert(true)

	for _, row := range rows {
		brandID := "brand_" + slugify(row.marca)
		if _, err := brands.UpdateOne(ctx, bson.M{"_id": brandID}, bson.M{"$set": bson.M{
			"name":     row.marca,
			"is_other": false,
		}}, upsertOpts); err != nil {
			log.Fatalf("upsert brand %s: %v", brandID, err)
		}

		modelID := "model_" + slugify(row.marca) + "_" + slugify(row.modelo)
		if _, err := modelsCol.UpdateOne(ctx, bson.M{"_id": modelID}, bson.M{"$set": bson.M{
			"brand_id":     brandID,
			"name":         row.modelo,
			"vehicle_type": row.tipo,
			"is_other":     false,
		}}, upsertOpts); err != nil {
			log.Fatalf("upsert model %s: %v", modelID, err)
		}
		fmt.Printf("upserted %s / %s (%s)\n", row.marca, row.modelo, row.tipo)
	}

	if _, err := brands.UpdateOne(ctx, bson.M{"_id": models.VehicleBrandOutroID}, bson.M{"$set": bson.M{
		"name":     "Outro",
		"is_other": true,
	}}, upsertOpts); err != nil {
		log.Fatalf("upsert brand_outro: %v", err)
	}
	if _, err := modelsCol.UpdateOne(ctx, bson.M{"_id": models.VehicleModelOutroID}, bson.M{"$set": bson.M{
		"brand_id":     models.VehicleBrandOutroID,
		"name":         "Outro",
		"vehicle_type": models.VehicleTypeAutopropelido,
		"is_other":     true,
	}}, upsertOpts); err != nil {
		log.Fatalf("upsert model_outro: %v", err)
	}
	fmt.Printf("upserted sentinel %s / %s (is_other=true)\n", models.VehicleBrandOutroID, models.VehicleModelOutroID)
	fmt.Println("done")
}

type catalogRow struct {
	marca  string
	modelo string
	tipo   models.VehicleType
}

func readCatalogCSV(path string) ([]catalogRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("csv must have header + at least one row")
	}
	header := records[0]
	if len(header) < 3 || !strings.EqualFold(header[0], "marca") || !strings.EqualFold(header[1], "modelo") || !strings.EqualFold(header[2], "tipo") {
		return nil, fmt.Errorf("expected header marca,modelo,tipo")
	}

	var rows []catalogRow
	for i, rec := range records[1:] {
		if len(rec) < 3 {
			return nil, fmt.Errorf("row %d: expected 3 columns", i+2)
		}
		tipo := models.VehicleType(strings.TrimSpace(rec[2]))
		if !models.IsValidVehicleType(tipo) {
			return nil, fmt.Errorf("row %d: invalid tipo %q", i+2, rec[2])
		}
		rows = append(rows, catalogRow{
			marca:  strings.TrimSpace(rec[0]),
			modelo: strings.TrimSpace(rec[1]),
			tipo:   tipo,
		})
	}
	return rows, nil
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_':
			b.WriteByte('_')
		}
	}
	return strings.Trim(nonAlnum.ReplaceAllString(b.String(), "_"), "_")
}
