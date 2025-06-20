package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	// "log"
	"math/rand/v2"
	"net/http"
	// "os"
	"sync"
	"time"
)

type SeatList []int

type ReserveRequest struct {
	UserID int `json:"user_id"`
	SeatID int `json:"seat_id"`
}

type Result struct {
	StatusCode int
	Duration   time.Duration
	Err        error
}

const (
	concurrentClients = 5000
	loadURL           = "http://server:8080/seats/available"
	reserveURL        = "http://server:8080/reserve"
)

func fetchAvailableSeats(client *http.Client) (SeatList, error) {
	resp, err := client.Get(loadURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var seats SeatList
	if err := json.NewDecoder(resp.Body).Decode(&seats); err != nil {
		return nil, err
	}

	return seats, nil
}

func tryReserve(client *http.Client, req ReserveRequest) Result {
	body, _ := json.Marshal(req)
	start := time.Now()
	resp, err := client.Post(reserveURL, "application/json", bytes.NewBuffer(body))
	duration := time.Since(start)

	if err != nil {
		return Result{StatusCode: 0, Duration: duration, Err: err}
	}
	defer resp.Body.Close()

	return Result{StatusCode: resp.StatusCode, Duration: duration}
}

func simulateClient(userID int, client *http.Client, wg *sync.WaitGroup, results chan<- []Result) {
	defer wg.Done()

	currentResults := make([]Result, 0)

	for {
		seats, err := fetchAvailableSeats(client)
		if err != nil {
			continue
		}

		if len(seats) == 0 {
			break
		}

		// 좌석 셔플
		rand.Shuffle(len(seats), func(i, j int) {
			seats[i], seats[j] = seats[j], seats[i]
		})

		for i := 0; i < len(seats) && i < 3; i++ {
			seatID := seats[i]

			// 측정 대상: 딱 한 번의 리퀘스트-리스폰 시간
			result := tryReserve(client, ReserveRequest{
				UserID: userID,
				SeatID: seatID,
			})

			// 네트워크 오류면 아예 통계 제외
			if result.Err != nil || result.Duration == 0 {
				continue
			}

			currentResults = append(currentResults, result)

			if result.StatusCode == http.StatusOK {
				break
			}

			time.Sleep(time.Duration(int(rand.Float64()*100)) * time.Millisecond)
		}
	}

	if len(currentResults) == 0 {
		currentResults = append(currentResults, Result{
			StatusCode: 0,
			Err:        fmt.Errorf("user %d: no request succeeded", userID),
			Duration:   0,
		})
	}

	// 결과 전송 (성공 or 실패 관계없이)
	results <- currentResults
}

func main() {
	var wg sync.WaitGroup
	results := make(chan []Result, concurrentClients)
	client := &http.Client{Timeout: 5 * time.Second}

	fmt.Println("Starting load test...")
	time.Sleep(10 * time.Second) // 서버 안정화 대기

	for i := 0; i < concurrentClients; i++ {
		wg.Add(1)
		go simulateClient(1000+i, client, &wg, results)
	}

	wg.Wait()
	close(results)

	var (
		successCount    int
		successTotalRTT time.Duration

		failCount    int
		failTotalRTT time.Duration

		requestFailCount int
	)
	var allResults []Result
	for rr := range results {
		for _, r := range rr {
			allResults = append(allResults, r)

			if r.Duration == 0 {
				// 네트워크 실패 (요청 자체가 실패했음)
				requestFailCount++
				continue
			}

			if r.StatusCode == http.StatusOK {
				// 예매 성공
				successCount++
				successTotalRTT += r.Duration
			} else {
				// 예매 실패 (응답은 옴)
				failCount++
				failTotalRTT += r.Duration
			}
		}
	}

	// 평균 계산
	// var (
	// 	successAvgRTT time.Duration
	// 	failAvgRTT    time.Duration
	// )

	// if successCount > 0 {
	// 	successAvgRTT = successTotalRTT / time.Duration(successCount)
	// }

	// if failCount > 0 {
	// 	failAvgRTT = failTotalRTT / time.Duration(failCount)
	// }

	// result := ""

	// 출력
	// fmt.Println("✅ Detailed Load Test Results")
	// result += "✅ Detailed Load Test Results\n"
	// fmt.Printf("Request Failures (no HTTP response): %d\n", requestFailCount)
	// result += fmt.Sprintf("Request Failures (no HTTP response): %d\n", requestFailCount)

	// fmt.Printf("Reservation Success: %d\n", successCount)
	// result += fmt.Sprintf("Reservation Success: %d\n", successCount)
	// fmt.Printf("  ↳ Avg RTT: %v\n", successAvgRTT)
	// result += fmt.Sprintf("  ↳ Avg RTT: %v\n", successAvgRTT)

	// fmt.Printf("Reservation Failure: %d\n", failCount)
	// result += fmt.Sprintf("Reservation Failure: %d\n", failCount)
	// fmt.Printf("  ↳ Avg RTT: %v\n", failAvgRTT)
	// result += fmt.Sprintf("  ↳ Avg RTT: %v\n", failAvgRTT)

	// f, err := os.OpenFile("/results/load_test_results.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0777)
	// if err != nil {
	// 	log.Fatalf("파일 열기 실패: %v", err)
	// }
	// defer f.Close()

	// if _, err := f.WriteString(result + "\n"); err != nil {
	// 	log.Fatalf("파일 쓰기 실패: %v", err)
	// }
}
