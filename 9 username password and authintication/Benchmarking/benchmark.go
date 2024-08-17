package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type BenchmarkResult struct {
	TotalRequests     int
	Successful        int
	Failed            int
	TotalTime         time.Duration
	MinResponseTime   time.Duration
	MaxResponseTime   time.Duration
	TotalResponseTime time.Duration
}

var result BenchmarkResult
var mu sync.Mutex

func main() {
	var port = flag.String("p", "9999", "server port to use")
	var host = flag.String("h", "teleport.me", "server hostname to use")
	var username = flag.String("u", "test1", "username for authentication")
	var password = flag.String("pwd", "123456", "password for authentication")
	var numRequests = flag.Int("n", 100, "number of requests to send")
	var logFile = flag.String("log", "benchmark.log", "log file to store results")
	flag.Parse()

	file, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()
	log.SetOutput(file)

	startTime := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < *numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			benchmarkRequest(*host, *port, *username, *password)
		}()
	}
	wg.Wait()

	endTime := time.Now()
	result.TotalTime = endTime.Sub(startTime)

	// Log final statistics
	logStatistics()
	fmt.Println("Benchmarking completed. See log file for details.")
}

func benchmarkRequest(host, port, username, password string) {
	startTime := time.Now()

	url := fmt.Sprintf("http://%s:%s/", host, port)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logError(err, "Creating HTTP request")
		return
	}

	req.Header.Set("X-Username", username)
	req.Header.Set("X-Password", password)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	responseTime := time.Since(startTime)

	if err != nil {
		logError(err, "Executing HTTP request")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		incrementSuccessfulRequests(responseTime)
	} else {
		log.Printf("Non-200 response: %s\n", resp.Status)
		incrementFailedRequests()
	}
}

func logError(err error, context string) {
	log.Printf("Error %s: %v\n", context, err)
	incrementFailedRequests()
}

func incrementSuccessfulRequests(responseTime time.Duration) {
	mu.Lock()
	defer mu.Unlock()

	result.Successful++
	result.TotalRequests++
	result.TotalResponseTime += responseTime

	if result.MinResponseTime == 0 || responseTime < result.MinResponseTime {
		result.MinResponseTime = responseTime
	}
	if responseTime > result.MaxResponseTime {
		result.MaxResponseTime = responseTime
	}
}

func incrementFailedRequests() {
	mu.Lock()
	defer mu.Unlock()

	result.Failed++
	result.TotalRequests++
}

func logStatistics() {
	mu.Lock()
	defer mu.Unlock()

	averageResponseTime := time.Duration(0)
	if result.Successful > 0 {
		averageResponseTime = result.TotalResponseTime / time.Duration(result.Successful)
	}

	log.Printf("\n--- Benchmark Results ---\n")
	log.Printf("Total Requests: %d\n", result.TotalRequests)
	log.Printf("Successful Requests: %d\n", result.Successful)
	log.Printf("Failed Requests: %d\n", result.Failed)
	log.Printf("Min Response Time: %s\n", result.MinResponseTime)
	log.Printf("Max Response Time: %s\n", result.MaxResponseTime)
	log.Printf("Average Response Time: %s\n", averageResponseTime)
	log.Printf("Total Benchmark Time: %s\n", result.TotalTime)
	log.Printf("--------------------------\n")
}
