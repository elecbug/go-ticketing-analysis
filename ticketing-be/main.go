package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type TicketRequest struct {
	UserID int `json:"user_id"`
	SeatID int `json:"seat_id"`
}

var db *sql.DB

// 좌석 리스트 반환 (100개 제한)
func availableSeatsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT seat_id FROM seats WHERE status = 'available' ORDER BY seat_id LIMIT 100`)
	if err != nil {
		log.Printf("DB error in availableSeatsHandler: %v", err)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(seats)
}

// 좌석 예매 처리
func reserveHandler(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	var req TicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("DB begin error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback() // 안전하게 Rollback 선언

	var status string
	err = tx.QueryRow(`SELECT status FROM seats WHERE seat_id = ? FOR UPDATE`, req.SeatID).Scan(&status)
	if err == sql.ErrNoRows {
		http.Error(w, "Seat not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("DB select error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if status != "available" {
		http.Error(w, "Seat already reserved", http.StatusConflict)
		return
	}

	_, err = tx.Exec(`UPDATE seats SET status = 'reserved', user_id = ? WHERE seat_id = ?`, req.UserID, req.SeatID)
	if err != nil {
		log.Printf("DB update error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("DB commit error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

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
		return err
	}

	for i := 1; i <= total; i++ {
		_, err := db.Exec(`INSERT IGNORE INTO seats (seat_id) VALUES (?)`, i)
		if err != nil {
			log.Printf("Insert seat %d failed: %v", i, err)
		}
	}

	return nil
}

func main() {
	var err error
	dsn := "root:password@tcp(db:3306)/ticketing"
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}

	// 커넥션 풀 설정 (실험환경 대응)
	db.SetMaxOpenConns(1000)
	db.SetMaxIdleConns(100)
	db.SetConnMaxLifetime(30 * time.Second)

	// DB 연결 대기
	for {
		if err = db.Ping(); err != nil {
			log.Printf("DB not reachable: %v", err)
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}

	if err := initSeats(10000); err != nil {
		log.Fatalf("Seat initialization failed: %v", err)
	}

	http.HandleFunc("/seats/available", availableSeatsHandler)
	http.HandleFunc("/reserve", reserveHandler)

	fmt.Println("Ticketing server is running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
