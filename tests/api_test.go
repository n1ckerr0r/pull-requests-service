package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

const base = "http://localhost:8080"

func connectTestDB(t *testing.T) *sqlx.DB {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:postgres@localhost:55432/pr_service?sslmode=disable"
	}

	db, err := sqlx.Open("postgres", url)
	if err != nil {
		t.Fatalf("failed to connect to DB: %v", err)
	}

	if err = db.Ping(); err != nil {
		t.Fatalf("failed to ping DB: %v", err)
	}

	return db
}

func resetDatabase(t *testing.T, db *sqlx.DB) {
	_, err := db.Exec(`
        TRUNCATE TABLE pr_assignments RESTART IDENTITY CASCADE;
        TRUNCATE TABLE prs RESTART IDENTITY CASCADE;
        TRUNCATE TABLE users RESTART IDENTITY CASCADE;
        TRUNCATE TABLE teams RESTART IDENTITY CASCADE;
`)
	if err != nil {
		t.Fatalf("failed to reset DB: %v", err)
	}
}

func TestHealth(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("health error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestTeamLifecycle(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	body := []byte(`{
       "team_name": "qa",
       "members": [
          {"user_id": "u10", "username": "John", "is_active": true},
          {"user_id": "u11", "username": "Kate", "is_active": true}
       ]
    }`)

	resp, err := http.Post(base+"/team/add", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("team add error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	resp, err = http.Get(base + "/team/get?team_name=qa")
	if err != nil {
		t.Fatalf("team get error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestUserActivation(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	teamBody := []byte(`{
       "team_name": "backend",
       "members": [
          {"user_id": "user1", "username": "Mike", "is_active": true}
       ]
    }`)

	resp, err := http.Post(base+"/team/add", "application/json", bytes.NewReader(teamBody))
	if err != nil {
		t.Fatalf("team add error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	deactivateBody := []byte(`{
       "user_id": "user1",
       "is_active": false
    }`)

	resp, err = http.Post(base+"/users/setIsActive", "application/json", bytes.NewReader(deactivateBody))
	if err != nil {
		t.Fatalf("deactivate user error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp, err = http.Get(base + "/team/get?team_name=backend")
	if err != nil {
		t.Fatalf("team get error: %v", err)
	}
	defer resp.Body.Close()

	var teamResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&teamResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	members, ok := teamResp["members"].([]interface{})
	if !ok {
		t.Fatalf("invalid members format in response")
	}

	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}

	member, ok := members[0].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid member format")
	}

	if member["is_active"] != false {
		t.Fatalf("expected user to be inactive")
	}
}

func TestReassignScenarios(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	teamBody := []byte(`{
       "team_name": "frontend",
       "members": [
          {"user_id": "f1", "username": "Front1", "is_active": true},
          {"user_id": "f2", "username": "Front2", "is_active": true},
          {"user_id": "f3", "username": "Front3", "is_active": false}
       ]
    }`)

	resp, err := http.Post(base+"/team/add", "application/json", bytes.NewReader(teamBody))
	if err != nil {
		t.Fatalf("team add error: %v", err)
	}
	defer resp.Body.Close()

	prBody := []byte(`{
       "pull_request_id": "pr-front",
       "pull_request_name": "Frontend Feature",
       "author_id": "f1"
    }`)

	resp, err = http.Post(base+"/pullRequest/create", "application/json", bytes.NewReader(prBody))
	if err != nil {
		t.Fatalf("pr create error: %v", err)
	}
	defer resp.Body.Close()

	var prResp map[string]interface{}
	if jsonErr := json.NewDecoder(resp.Body).Decode(&prResp); jsonErr != nil {
		t.Fatalf("failed to decode PR response: %v", err)
	}

	pr, ok := prResp["pr"].(map[string]interface{})
	if !ok {
		t.Fatalf("invalid PR format in response")
	}

	reviewers, ok := pr["assigned_reviewers"].([]interface{})
	if !ok {
		t.Fatalf("invalid reviewers format")
	}

	if len(reviewers) != 1 {
		t.Fatalf("expected 1 reviewer, got %d", len(reviewers))
	}

	reassignBody := []byte(`{
       "pull_request_id": "pr-front",
       "old_user_id": "nonexistent"
    }`)
	resp, err = http.Post(base+"/pullRequest/reassign", "application/json", bytes.NewReader(reassignBody))
	if err != nil {
		t.Fatalf("reassign error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 && resp.StatusCode != 409 {
		t.Fatalf("expected 404 or 409, got %d", resp.StatusCode)
	}
}

func TestDuplicateTeam(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	teamBody := []byte(`{
       "team_name": "duplicate",
       "members": [{"user_id": "d1", "username": "Duplicate", "is_active": true}]
    }`)

	resp, err := http.Post(base+"/team/add", "application/json", bytes.NewReader(teamBody))
	if err != nil {
		t.Fatalf("team add error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	resp, err = http.Post(base+"/team/add", "application/json", bytes.NewReader(teamBody))
	if err != nil {
		t.Fatalf("team add error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("expected 400 for duplicate team, got %d", resp.StatusCode)
	}
}

func TestGetUserReviews(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	teamBody := []byte(`{
       "team_name": "reviews",
       "members": [
          {"user_id": "r1", "username": "Reviewer1", "is_active": true},
          {"user_id": "r2", "username": "Reviewer2", "is_active": true},
          {"user_id": "r3", "username": "Author", "is_active": true}
       ]
    }`)

	resp, err := http.Post(base+"/team/add", "application/json", bytes.NewReader(teamBody))
	if err != nil {
		t.Fatalf("team add error: %v", err)
	}
	defer resp.Body.Close()

	prs := []string{"pr-review-1", "pr-review-2"}
	for _, prID := range prs {
		prBody := []byte(`{
          "pull_request_id": "` + prID + `",
          "pull_request_name": "Review Test",
          "author_id": "r3"
       }`)

		resp, err = http.Post(base+"/pullRequest/create", "application/json", bytes.NewReader(prBody))
		if err != nil {
			t.Fatalf("pr create error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 201 {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}
	}

	resp, err = http.Get(base + "/users/getReview?user_id=r1")
	if err != nil {
		t.Fatalf("get review error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var reviewsResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&reviewsResp); err != nil {
		t.Fatalf("failed to decode reviews response: %v", err)
	}

	prsList, ok := reviewsResp["pull_requests"].([]interface{})
	if !ok {
		t.Fatalf("invalid pull_requests format in response")
	}

	if len(prsList) < 1 {
		t.Fatalf("expected at least 1 PR for review, got %d", len(prsList))
	}
}

func TestNonexistentEntities(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	resp, err := http.Get(base + "/team/get?team_name=nonexistent")
	if err != nil {
		t.Fatalf("team get error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 for nonexistent team, got %d", resp.StatusCode)
	}

	prBody := []byte(`{
       "pull_request_id": "pr-ghost",
       "pull_request_name": "Ghost PR",
       "author_id": "ghost"
    }`)

	resp, err = http.Post(base+"/pullRequest/create", "application/json", bytes.NewReader(prBody))
	if err != nil {
		t.Fatalf("pr create error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 for nonexistent author, got %d", resp.StatusCode)
	}
}

func createTestDataForLoad(_ *testing.T, _ *sqlx.DB) {
	teamBody := []byte(`{
       "team_name": "load_team_base",
       "members": [
          {"user_id": "user_0_0", "username": "Load User 0", "is_active": true},
          {"user_id": "user_0_1", "username": "Load User 1", "is_active": true},
          {"user_id": "user_0_2", "username": "Load User 2", "is_active": true},
          {"user_id": "user_0_3", "username": "Load User 3", "is_active": true},
          {"user_id": "user_0_4", "username": "Load User 4", "is_active": true},
          {"user_id": "user_0_5", "username": "Load User 5", "is_active": false}
       ]
    }`)

	resp, err := http.Post(base+"/team/add", "application/json", bytes.NewReader(teamBody))
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func TestLoadCreateTeamsAndUsers(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	teamCount := 50
	usersPerTeam := 10
	successCount := 0
	var mutex sync.Mutex

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)

	for i := 0; i < teamCount; i++ {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(teamIndex int) {
			defer wg.Done()
			defer func() { <-semaphore }()

			teamName := fmt.Sprintf("load_team_%d", teamIndex)
			members := make([]map[string]interface{}, usersPerTeam)

			for j := 0; j < usersPerTeam; j++ {
				members[j] = map[string]interface{}{
					"user_id":   fmt.Sprintf("user_%d_%d", teamIndex, j),
					"username":  fmt.Sprintf("User %d-%d", teamIndex, j),
					"is_active": j < 8, // 80% активных пользователей
				}
			}

			body := map[string]interface{}{
				"team_name": teamName,
				"members":   members,
			}

			jsonBody, err := json.Marshal(body)
			if err != nil {
				t.Logf("JSON marshal failed: %v", err)
				return
			}

			start := time.Now()
			resp, err := http.Post(base+"/team/add", "application/json", bytes.NewReader(jsonBody))
			elapsed := time.Since(start)

			if err != nil {
				t.Logf("Team creation failed: %v", err)
				return
			}
			defer resp.Body.Close()

			mutex.Lock()
			if resp.StatusCode == 201 {
				successCount++
			}
			mutex.Unlock()

			if elapsed > time.Second {
				t.Logf("Slow team creation: %v", elapsed)
			}
		}(i)
	}

	wg.Wait()
	t.Logf("Created %d/%d teams with %d users each", successCount, teamCount, usersPerTeam)
}

func TestLoadPRCreation(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	createTestDataForLoad(t, db)

	prCount := 100
	successCount := 0
	var totalResponseTime time.Duration
	var responseTimeMutex sync.Mutex

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 20)

	for i := 0; i < prCount; i++ {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(prIndex int) {
			defer wg.Done()
			defer func() { <-semaphore }()

			authorID := fmt.Sprintf("user_0_%d", rand.Intn(5))
			prID := fmt.Sprintf("load_pr_%d_%d", time.Now().UnixNano(), prIndex)

			body := map[string]interface{}{
				"pull_request_id":   prID,
				"pull_request_name": fmt.Sprintf("Load Test PR %d", prIndex),
				"author_id":         authorID,
			}

			jsonBody, err := json.Marshal(body)
			if err != nil {
				t.Logf("JSON marshal failed: %v", err)
				return
			}

			start := time.Now()
			resp, err := http.Post(base+"/pullRequest/create", "application/json", bytes.NewReader(jsonBody))
			elapsed := time.Since(start)

			if err != nil {
				t.Logf("PR creation failed: %v", err)
				return
			}
			defer resp.Body.Close()

			responseTimeMutex.Lock()
			totalResponseTime += elapsed
			if resp.StatusCode == 201 {
				successCount++
			}
			responseTimeMutex.Unlock()

			if elapsed > 2*time.Second {
				t.Logf("Slow PR creation: %v for PR %s", elapsed, prID)
			}

			time.Sleep(10 * time.Millisecond)
		}(i)
	}

	wg.Wait()

	avgResponseTime := totalResponseTime / time.Duration(prCount)
	t.Logf("PR Creation Load Test: %d/%d successful, avg response time: %v",
		successCount, prCount, avgResponseTime)

	if avgResponseTime > 500*time.Millisecond {
		t.Errorf("Average response time too high: %v", avgResponseTime)
	}
}

func TestLoadDatabaseConnections(t *testing.T) {
	db := connectTestDB(t)
	resetDatabase(t, db)

	createTestDataForLoad(t, db)

	concurrentConnections := 20
	successCount := 0
	var successMutex sync.Mutex

	var wg sync.WaitGroup

	for i := 0; i < concurrentConnections; i++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()

			localSuccess := true
			for j := 0; j < 10; j++ {
				userID := fmt.Sprintf("user_0_%d", rand.Intn(5))
				resp, err := http.Get(base + "/users/getReview?user_id=" + userID)
				if err != nil || resp.StatusCode != 200 {
					localSuccess = false
				}
				if resp != nil {
					resp.Body.Close()
				}
				time.Sleep(time.Duration(10+rand.Intn(40)) * time.Millisecond)
			}

			if localSuccess {
				successMutex.Lock()
				successCount++
				successMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Database Connections Test: %d/%d successful concurrent connections",
		successCount, concurrentConnections)

	if successCount < concurrentConnections*8/10 {
		t.Errorf("Too many failed concurrent connections: %d/%d",
			concurrentConnections-successCount, concurrentConnections)
	}
}
