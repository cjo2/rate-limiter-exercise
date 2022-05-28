package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type RateLimiter struct {
	slotsLock          sync.Mutex
	slots              int
	RateLimitDataLock  sync.Mutex
	RateLimit          int
	RateLimitReset     int64
	RateLimitRemaining int
}

func (r *RateLimiter) UpdateRateLimit(limit int) {
	r.RateLimitDataLock.Lock()
	defer r.RateLimitDataLock.Unlock()
	r.RateLimit = limit
}

func (r *RateLimiter) UpdateRateLimitResetTime(timestamp int) {
	r.RateLimitDataLock.Lock()
	defer r.RateLimitDataLock.Unlock()
	r.RateLimitReset = int64(timestamp)
}

func (r *RateLimiter) UpdateRateLimitRemaining(remaining int) {
	r.RateLimitDataLock.Lock()
	defer r.RateLimitDataLock.Unlock()
	r.RateLimitRemaining = remaining
}

func (r *RateLimiter) CompareAndSetRateLimitData(timestamp int, limit int, limitRemaining int) {
	r.RateLimitDataLock.Lock()
	defer r.RateLimitDataLock.Unlock()
	if int64(timestamp) > r.RateLimitReset || limit < r.RateLimit || limitRemaining < r.RateLimitRemaining {
		r.RateLimitReset = int64(timestamp)
		r.RateLimit = limit
		r.RateLimitRemaining = limitRemaining
	}
}

func (r *RateLimiter) getRateLimitResetTime() int64 {
	r.RateLimitDataLock.Lock()
	defer r.RateLimitDataLock.Unlock()
	return r.RateLimitReset
}

func (r *RateLimiter) GetSlot() bool {
	r.slotsLock.Lock()
	defer r.slotsLock.Unlock()

	if r.slots == 0 {
		return false
	}

	r.slots = r.slots - 1
	return true
}

func (r *RateLimiter) ReleaseSlot(count int) {
	r.slotsLock.Lock()
	defer r.slotsLock.Unlock()

	r.slots = r.slots + count
}

func MakeRequestWithRateLimit(client *http.Client, rateLimiter *RateLimiter, url string) (bool, error) {

	resp, err := client.Get(url)

	if err != nil {
		log.Fatal(err)
	}

	log.Println(resp)

	defer resp.Body.Close()

	resetString := resp.Header.Get("RateLimit-Reset")
	resetTime, err := strconv.Atoi(resetString)
	if err != nil {
		log.Fatal("Could not read reset time from header")
	}

	limitString := resp.Header.Get("RateLimit-Limit")
	limit, err := strconv.Atoi(limitString)
	if err != nil {
		log.Fatal("Could not read limit from header")
	}

	limitRemainingString := resp.Header.Get("RateLimit-Remaining")
	limitRemaining, err := strconv.Atoi(limitRemainingString)
	if err != nil {
		log.Fatal("Could not read remaining")
	}

	rateLimiter.CompareAndSetRateLimitData(resetTime, limit, limitRemaining)

	return true, nil
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{slots: 50}
}

func main() {
	url := os.Args[1]
	rateLimiter := NewRateLimiter()
	tr := &http.Transport{
		MaxIdleConns:       0,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: true,
		MaxConnsPerHost:    2,
	}
	client := &http.Client{Transport: tr}
	for {
		slotRetrieved := false

		for !slotRetrieved {
			slotRetrieved = rateLimiter.GetSlot()

			if rateLimiter.RateLimitReset != 0 && !slotRetrieved {
				reset := time.Unix(rateLimiter.getRateLimitResetTime(), 0)
				until := time.Until(reset)
				time.Sleep(until)
				rateLimiter.ReleaseSlot(50)
			}
		}

		if rateLimiter.RateLimitReset == 0 {
			MakeRequestWithRateLimit(client, rateLimiter, url)
		} else {
			// This should be asynchronous
			go MakeRequestWithRateLimit(client, rateLimiter, url)
		}
	}
}
