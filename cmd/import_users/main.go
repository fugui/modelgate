package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"modelgate/internal/config"
	"modelgate/internal/infra/auth"
	"modelgate/internal/infra/db"
	"modelgate/internal/repository"
)

func main() {
	var configPath string
	var csvPath string

	flag.StringVar(&configPath, "config", "config.yaml", "Path to config file")
	flag.StringVar(&csvPath, "csv", "", "Path to CSV file containing users")
	flag.Parse()

	if csvPath == "" {
		fmt.Println("Usage: import_users -csv <path_to_csv> [-config <path_to_config>]")
		fmt.Println("CSV format expects a header row: email,password,name,role,department,quota_policy")
		os.Exit(1)
	}

	// Load Config
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to Database
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	userStore := entity.NewUserStore(database.DB)

	// Open CSV File
	file, err := os.Open(csvPath)
	if err != nil {
		log.Fatalf("Failed to open CSV file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("Failed to parse CSV file: %v", err)
	}

	if len(records) < 2 {
		log.Fatal("CSV file must contain a header row and at least one user record")
	}

	// Extract headers
	headers := records[0]
	headerMap := make(map[string]int)
	for i, h := range headers {
		headerMap[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// Check required fields
	reqFields := []string{"email", "password", "name", "role"}
	for _, field := range reqFields {
		if _, ok := headerMap[field]; !ok {
			log.Fatalf("CSV header missing required field: %s", field)
		}
	}

	successCount := 0
	errorCount := 0

	for i, record := range records[1:] {
		rowNum := i + 2 // 1-indexed, starting from row 2

		email := strings.TrimSpace(record[headerMap["email"]])
		password := strings.TrimSpace(record[headerMap["password"]])
		name := strings.TrimSpace(record[headerMap["name"]])
		roleStr := strings.TrimSpace(record[headerMap["role"]])

		department := ""
		if idx, ok := headerMap["department"]; ok && idx < len(record) {
			department = strings.TrimSpace(record[idx])
		}

		quotaPolicy := "default"
		if idx, ok := headerMap["quota_policy"]; ok && idx < len(record) {
			val := strings.TrimSpace(record[idx])
			if val != "" {
				quotaPolicy = val
			}
		}

		if email == "" || password == "" || name == "" {
			log.Printf("Row %d: Missing required fields (email, password, or name). Skipping.", rowNum)
			errorCount++
			continue
		}

		// Role validation
		role := entity.RoleUser
		switch strings.ToLower(roleStr) {
		case "admin":
			role = entity.RoleAdmin
		case "manager":
			role = entity.RoleManager
		case "user":
			role = entity.RoleUser
		default:
			log.Printf("Row %d: Invalid role '%s', defaulting to 'user'", rowNum, roleStr)
		}

		// Check if user already exists
		existing, err := userStore.GetByEmail(email)
		if err != nil {
			log.Printf("Row %d: Database error checking existing user: %v", rowNum, err)
			errorCount++
			continue
		}
		if existing != nil {
			log.Printf("Row %d: User with email %s already exists. Skipping.", rowNum, email)
			errorCount++
			continue
		}

		// Hash password
		passwordHash, err := auth.HashPassword(password)
		if err != nil {
			log.Printf("Row %d: Failed to hash password: %v", rowNum, err)
			errorCount++
			continue
		}

		// Create user entity
		user := &entity.User{
			Email:        email,
			PasswordHash: passwordHash,
			Name:         name,
			Role:         role,
			Department:   department,
			QuotaPolicy:  quotaPolicy,
			Enabled:      true,
		}

		// Save to DB
		if err := userStore.Create(user); err != nil {
			log.Printf("Row %d: Failed to create user in database: %v", rowNum, err)
			errorCount++
			continue
		}

		log.Printf("Row %d: Successfully imported user %s", rowNum, email)
		successCount++
	}

	fmt.Printf("\n--- Import Complete ---\n")
	fmt.Printf("Total Source Records: %d\n", len(records)-1)
	fmt.Printf("Successfully Imported: %d\n", successCount)
	fmt.Printf("Errors/Skipped: %d\n", errorCount)
}
