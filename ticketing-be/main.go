package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// JSON 형식 로그 구조체
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Action    string `json:"action"`
	UserID    int    `json:"user_id,omitempty"`
	SeatID    int    `json:"seat_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
}

// JSON 로그 출력 함수
func logJSON(level, action string, userID, seatID int, status string, err error) {
	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level,
		Action:    action,
		UserID:    userID,
		SeatID:    seatID,
		Status:    status,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	data, _ := json.Marshal(entry)
	log.Println(string(data))
}

type TicketRequest struct {
	UserID int `json:"user_id"`
	SeatID int `json:"seat_id"`
}

var db *sql.DB

// 좌석 리스트 반환
func availableSeatsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT seat_id FROM seats WHERE status = 'available' ORDER BY seat_id`)
	if err != nil {
		logJSON("ERROR", "available_seats", 0, 0, "query_fail", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var seats []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			seats = append(seats, id)
		}
	}

	logJSON("INFO", "available_seats", 0, 0, fmt.Sprintf("count=%d", len(seats)), nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(seats)
}

// 좌석 예매 처리
func reserveHandler(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		logJSON("WARN", "reserve", 0, 0, "bad_content_type", nil)
		return
	}

	var req TicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		logJSON("ERROR", "reserve", 0, 0, "invalid_json", err)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		logJSON("ERROR", "reserve", req.UserID, req.SeatID, "tx_begin_fail", err)
		return
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRow(`SELECT status FROM seats WHERE seat_id = ? FOR UPDATE`, req.SeatID).Scan(&status)
	if err == sql.ErrNoRows {
		http.Error(w, "Seat not found", http.StatusNotFound)
		logJSON("WARN", "reserve", req.UserID, req.SeatID, "seat_not_found", nil)
		return
	} else if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		logJSON("ERROR", "reserve", req.UserID, req.SeatID, "select_fail", err)
		return
	}

	if status != "available" {
		http.Error(w, "Seat already reserved", http.StatusConflict)
		logJSON("INFO", "reserve", req.UserID, req.SeatID, "seat_conflict", nil)
		return
	}

	_, err = tx.Exec(`UPDATE seats SET status = 'reserved', user_id = ? WHERE seat_id = ?`, req.UserID, req.SeatID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		logJSON("ERROR", "reserve", req.UserID, req.SeatID, "update_fail", err)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		logJSON("ERROR", "reserve", req.UserID, req.SeatID, "commit_fail", err)
		return
	}

	logJSON("INFO", "reserve", req.UserID, req.SeatID, "success", nil)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Reservation successful",
	})
}

// 좌석 테이블 생성 및 초기화
func initSeats(total int) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS seats (
			seat_id INT PRIMARY KEY,
			status VARCHAR(20) NOT NULL DEFAULT 'available',
			user_id INT
		)
	`)
	if err != nil {
		logJSON("ERROR", "init_seats", 0, 0, "create_table_fail", err)
		return err
	}

	for i := 1; i <= total; i++ {
		_, err := db.Exec(`INSERT IGNORE INTO seats (seat_id) VALUES (?)`, i)
		if err != nil {
			logJSON("WARN", "init_seats", 0, i, "insert_ignore_fail", err)
		}
	}

	logJSON("INFO", "init_seats", 0, 0, fmt.Sprintf("inserted_up_to=%d", total), nil)
	return nil
}

func main() {
	var err error

	logFile, err := os.OpenFile("/results/ticketing.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	log.SetOutput(logFile)

	dsn := "root:password@tcp(db:3306)/ticketing"
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		logJSON("FATAL", "main", 0, 0, "db_open_fail", err)
		log.Fatalf("Failed to open DB: %v", err)
	}

	db.SetMaxOpenConns(1000)
	db.SetMaxIdleConns(100)
	db.SetConnMaxLifetime(30 * time.Second)

	for {
		if err = db.Ping(); err != nil {
			logJSON("WARN", "main", 0, 0, "db_not_reachable", err)
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}
	logJSON("INFO", "main", 0, 0, "db_connected", nil)

	if err := initSeats(10000); err != nil {
		logJSON("FATAL", "main", 0, 0, "seat_init_fail", err)
		log.Fatalf("Seat initialization failed: %v", err)
	}

	http.HandleFunc("/seats/available", availableSeatsHandler)
	http.HandleFunc("/reserve", reserveHandler)

	logJSON("INFO", "main", 0, 0, "server_start", nil)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
