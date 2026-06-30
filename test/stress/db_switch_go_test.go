package stress

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prest/prest/v2/config"
	"github.com/prest/prest/v2/router"
)

// TestMultiDatabaseSwitching tests concurrent database switching
func TestMultiDatabaseSwitching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	config.Load()
	
	databases := []string{"plano", "kratos"}
	iterations := 100
	concurrent := 10
	
	var successCount int64
	var failCount int64
	
	t.Run("sequential_switching", func(t *testing.T) {
		for i := 0; i < iterations; i++ {
			for _, db := range databases {
				req := httptest.NewRequest("GET", fmt.Sprintf("/%s/public", db), nil)
				w := httptest.NewRecorder()
				
				router := router.GetRouter()
				router.ServeHTTP(w, req)
				
				if w.Code == http.StatusOK {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&failCount, 1)
				}
			}
		}
		
		t.Logf("Sequential: Success=%d, Fail=%d", successCount, failCount)
	})
	
	successCount = 0
	failCount = 0
	
	t.Run("concurrent_switching", func(t *testing.T) {
		var wg sync.WaitGroup
		
		for i := 0; i < concurrent; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				
				for j := 0; j < iterations; j++ {
					db := databases[j%len(databases)]
					req := httptest.NewRequest("GET", fmt.Sprintf("/%s/public", db), nil)
					w := httptest.NewRecorder()
					
					router := router.GetRouter()
					router.ServeHTTP(w, req)
					
					if w.Code == http.StatusOK {
						atomic.AddInt64(&successCount, 1)
					} else {
						atomic.AddInt64(&failCount, 1)
					}
				}
			}(i)
		}
		
		wg.Wait()
		
		total := successCount + failCount
		t.Logf("Concurrent: Total=%d, Success=%d, Fail=%d", total, successCount, failCount)
		
		if failCount > 0 {
			t.Errorf("%d requests failed out of %d", failCount, total)
		}
	})
}

// TestDatabaseSwitchingUnderLoad tests database switching under heavy load
func TestDatabaseSwitchingUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	config.Load()
	
	databases := []string{"plano", "kratos"}
	duration := 10 * time.Second
	concurrent := 20
	
	var successCount int64
	var failCount int64
	var requestCount int64
	
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	
	var wg sync.WaitGroup
	
	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for {
				select {
				case <-ctx.Done():
					return
				default:
					db := databases[atomic.AddInt64(&requestCount, 1)%int64(len(databases))]
					req := httptest.NewRequest("GET", fmt.Sprintf("/%s/public", db), nil)
					w := httptest.NewRecorder()
					
					router := router.GetRouter()
					router.ServeHTTP(w, req)
					
					if w.Code == http.StatusOK {
						atomic.AddInt64(&successCount, 1)
					} else {
						atomic.AddInt64(&failCount, 1)
					}
				}
			}
		}(i)
	}
	
	wg.Wait()
	
	total := successCount + failCount
	t.Logf("Load Test Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Concurrent workers: %d", concurrent)
	t.Logf("  Total requests: %d", total)
	t.Logf("  Successful: %d", successCount)
	t.Logf("  Failed: %d", failCount)
	t.Logf("  Requests/sec: %.2f", float64(total)/duration.Seconds())
	
	if failCount > 0 {
		t.Errorf("%d requests failed out of %d", failCount, total)
	}
}

// TestDatabaseIsolation tests that queries on one database don't affect another
func TestDatabaseIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	config.Load()
	
	iterations := 50
	
	var wg sync.WaitGroup
	errors := make(chan error, iterations*2)
	
	// Concurrent queries to different databases
	for i := 0; i < iterations; i++ {
		wg.Add(2)
		
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/plano/public", nil)
			w := httptest.NewRecorder()
			router := router.GetRouter()
			router.ServeHTTP(w, req)
			
			if w.Code != http.StatusOK {
				errors <- fmt.Errorf("plano query failed: %d", w.Code)
			}
		}()
		
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/kratos/public", nil)
			w := httptest.NewRecorder()
			router := router.GetRouter()
			router.ServeHTTP(w, req)
			
			if w.Code != http.StatusOK {
				errors <- fmt.Errorf("kratos query failed: %d", w.Code)
			}
		}()
	}
	
	wg.Wait()
	close(errors)
	
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	
	if len(errs) > 0 {
		t.Errorf("Database isolation test failed with %d errors", len(errs))
		for _, err := range errs {
			t.Log(err)
		}
	}
}
