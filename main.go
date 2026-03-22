package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"golang.org/x/oauth2"

	"portfolio/components/pages"
	"portfolio/components/partials"
	"portfolio/types"
)

/*
========================================
Main
========================================
*/

const careerStartYear = 2012

const (
	defaultLPSAPIBaseURL       = "https://lps-api-prod.lps-test.com"
	lpsSessionCookieName       = "lps_session"
	googleConnectionCookieName = "google_connection"
	googleOAuthStateCookieName = "google_oauth_state"
	defaultSessionTTL          = 12 * time.Hour
	googleConnectionCookieTTL  = 180 * 24 * time.Hour
	googleOAuthStateTTL        = 10 * time.Minute
)

type serverConfig struct {
	SessionKey                []byte
	LPSAPIBaseURL             string
	GoogleClientID            string
	GoogleClientSecret        string
	GoogleConnectionTableName string
}

type loginAttempt struct {
	Count       int
	WindowStart time.Time
}

type loginRateLimiter struct {
	mu          sync.Mutex
	maxAttempts int
	window      time.Duration
	attempts    map[string]loginAttempt
	stop        chan struct{}
	closeOnce   sync.Once
}

type googleConnectionRecord struct {
	ConnectionID    string    `dynamodbav:"connection_id"`
	TokenCiphertext string    `dynamodbav:"token_ciphertext"`
	CalendarID      string    `dynamodbav:"calendar_id"`
	CalendarSummary string    `dynamodbav:"calendar_summary"`
	CreatedAt       time.Time `dynamodbav:"created_at"`
	UpdatedAt       time.Time `dynamodbav:"updated_at"`
}

type googleConnectionStore interface {
	Delete(ctx context.Context, connectionID string) error
	Get(ctx context.Context, connectionID string) (*googleConnectionRecord, error)
	Put(ctx context.Context, record *googleConnectionRecord) error
}

type dynamoGoogleConnectionStore struct {
	client    *dynamodb.Client
	tableName string
}

type noopGoogleConnectionStore struct{}

type googleOAuthState struct {
	ConnectionID string    `json:"connection_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	State        string    `json:"state"`
}

type googleCalendarListResponse struct {
	Items []googleCalendar `json:"items"`
}

type googleCalendar struct {
	ID      string `json:"id"`
	Primary bool   `json:"primary"`
	Summary string `json:"summary"`
}

type googleEventDateTime struct {
	DateTime string `json:"dateTime"`
}

type googleEvent struct {
	Description        string              `json:"description,omitempty"`
	End                googleEventDateTime `json:"end"`
	ExtendedProperties struct {
		Private map[string]string `json:"private,omitempty"`
	} `json:"extendedProperties,omitempty"`
	ID       string              `json:"id,omitempty"`
	Location string              `json:"location,omitempty"`
	Source   *googleEventSource  `json:"source,omitempty"`
	Start    googleEventDateTime `json:"start"`
	Summary  string              `json:"summary"`
}

type googleEventSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type googleAPIError struct {
	StatusCode int
	Message    string
}

func (err *googleAPIError) Error() string {
	if err == nil {
		return ""
	}
	if err.Message != "" {
		return err.Message
	}
	return "google api request failed"
}

var (
	configData               = loadServerConfig()
	lpsHTTPClient            = &http.Client{Timeout: 15 * time.Second}
	soccerLoginAttempts      = newLoginRateLimiter(5, time.Minute)
	errSessionExpired        = errors.New("session expired")
	errPlayerSessionRequired = errors.New("an imported session is required for discovered players")
	errInvalidTeamSelection  = errors.New("one or more team IDs were invalid")
	errScheduleSelection     = errors.New("at least one team ID or discovered player is required")
	googleConnectionsMu      sync.RWMutex
	googleConnections        googleConnectionStore = noopGoogleConnectionStore{}
	googleOAuthAuthURL                             = "https://accounts.google.com/o/oauth2/auth"
	googleOAuthTokenURL                            = "https://oauth2.googleapis.com/token" //nolint:gosec // G101: public OAuth endpoint URL, not a credential
	googleCalendarAPIBaseURL                       = "https://www.googleapis.com/calendar/v3"
)

type lpsErrorKind string

const (
	lpsErrorMalformedToken lpsErrorKind = "malformed_token"
	lpsErrorUnauthorized   lpsErrorKind = "unauthorized"
	lpsErrorForbidden      lpsErrorKind = "forbidden"
	lpsErrorInvalidPlayer  lpsErrorKind = "invalid_player"
	lpsErrorInvalidTeam    lpsErrorKind = "invalid_team"
	lpsErrorUpstream       lpsErrorKind = "upstream"
)

type lpsFetchError struct {
	Kind       lpsErrorKind
	PlayerID   int
	StatusCode int
	Err        error
}

func (err *lpsFetchError) Error() string {
	if err == nil {
		return ""
	}
	if err.Err != nil {
		return err.Err.Error()
	}
	return "schedule fetch failed"
}

func (err *lpsFetchError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func newLPSFetchError(kind lpsErrorKind, playerID, statusCode int, format string, args ...any) error {
	return &lpsFetchError{
		Kind:       kind,
		PlayerID:   playerID,
		StatusCode: statusCode,
		Err:        fmt.Errorf(format, args...),
	}
}

func loadServerConfig() serverConfig {
	config := serverConfig{
		LPSAPIBaseURL:             strings.TrimSpace(os.Getenv("LPS_API_BASE_URL")),
		GoogleClientID:            strings.TrimSpace(os.Getenv("CLIENT_ID_KEY")),
		GoogleClientSecret:        strings.TrimSpace(os.Getenv("CLIENT_SECRET_KEY")),
		GoogleConnectionTableName: strings.TrimSpace(os.Getenv("GOOGLE_CONNECTION_TABLE_NAME")),
	}
	if config.LPSAPIBaseURL == "" {
		config.LPSAPIBaseURL = defaultLPSAPIBaseURL
	}
	validatedLPSAPIBaseURL, err := normalizeLPSAPIBaseURL(config.LPSAPIBaseURL)
	if err != nil {
		log.Printf("invalid LPS_API_BASE_URL; using default %q", defaultLPSAPIBaseURL)
		config.LPSAPIBaseURL = defaultLPSAPIBaseURL
	} else {
		config.LPSAPIBaseURL = validatedLPSAPIBaseURL
	}
	if (config.GoogleClientID != "" || config.GoogleClientSecret != "" || config.GoogleConnectionTableName != "") &&
		(config.GoogleClientID == "" || config.GoogleClientSecret == "" || config.GoogleConnectionTableName == "") {
		log.Printf("google calendar add disabled: CLIENT_ID_KEY, CLIENT_SECRET_KEY, and GOOGLE_CONNECTION_TABLE_NAME must all be configured")
	}

	keyHex := strings.TrimSpace(os.Getenv("LPS_SESSION_KEY"))
	if keyHex == "" {
		log.Printf("soccer auth disabled: LPS_SESSION_KEY is not configured")
		return config
	}

	decoded, err := hex.DecodeString(keyHex)
	if err != nil || len(decoded) != 32 {
		log.Printf("soccer auth disabled: LPS_SESSION_KEY must be a 64-character hex string")
		return config
	}

	config.SessionKey = decoded

	return config
}

func normalizeLPSAPIBaseURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = defaultLPSAPIBaseURL
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() || parsed.Hostname() == "" {
		return "", errors.New("LPS API base URL must be absolute")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("LPS API base URL cannot include credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !isLoopbackHost(parsed.Hostname())) {
		return "", errors.New("LPS API base URL must use https, or http on loopback only")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	parsedIP := net.ParseIP(strings.TrimSpace(host))
	return parsedIP != nil && parsedIP.IsLoopback()
}

func lpsAPIEndpoint(pathParts ...string) (string, error) {
	baseURL, err := normalizeLPSAPIBaseURL(configData.LPSAPIBaseURL)
	if err != nil {
		return "", err
	}
	return url.JoinPath(baseURL, pathParts...)
}

func newLPSAPIRequest(ctx context.Context, method, bearerToken string, pathParts ...string) (*http.Request, error) {
	endpoint, err := lpsAPIEndpoint(pathParts...)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil) //nolint:gosec // endpoint is rebuilt from normalizeLPSAPIBaseURL and validated before use.
	if err != nil {
		return nil, err
	}
	if err := validateLPSAPIRequest(req); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	return req, nil
}

func validateLPSAPIRequest(req *http.Request) error {
	if req == nil || req.URL == nil {
		return errors.New("LPS API request URL is required")
	}

	if req.URL.RawQuery != "" || req.URL.Fragment != "" {
		return errors.New("LPS API requests cannot include query or fragment")
	}
	return nil
}

func doLPSAPIRequest(req *http.Request) (*http.Response, error) {
	if err := validateLPSAPIRequest(req); err != nil {
		return nil, err
	}
	return lpsHTTPClient.Do(req) //nolint:gosec // Request URLs are rebuilt from normalizeLPSAPIBaseURL and revalidated here.
}

func publicBindEnabled() bool {
	value := strings.TrimSpace(os.Getenv("APP_BIND_ALL"))
	return strings.EqualFold(value, "1") || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func serverListenAddress() string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" {
		if publicBindEnabled() {
			host = "0.0.0.0"
		} else {
			host = "127.0.0.1"
		}
	}
	return net.JoinHostPort(host, port)
}

func localServerURL(listenAddress string) string {
	_, port, err := net.SplitHostPort(listenAddress)
	if err != nil || port == "" {
		return "http://localhost:8080"
	}
	return "http://localhost:" + port
}

func newGoogleConnectionStore(ctx context.Context, config *serverConfig) (googleConnectionStore, error) {
	if strings.TrimSpace(config.GoogleConnectionTableName) == "" {
		return noopGoogleConnectionStore{}, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &dynamoGoogleConnectionStore{
		client:    dynamodb.NewFromConfig(awsCfg),
		tableName: config.GoogleConnectionTableName,
	}, nil
}

func currentGoogleConnectionStore() googleConnectionStore {
	googleConnectionsMu.RLock()
	defer googleConnectionsMu.RUnlock()
	return googleConnections
}

func setGoogleConnectionStore(store googleConnectionStore) {
	googleConnectionsMu.Lock()
	googleConnections = store
	googleConnectionsMu.Unlock()
}

func (noopGoogleConnectionStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (noopGoogleConnectionStore) Get(_ context.Context, _ string) (*googleConnectionRecord, error) {
	return nil, nil
}

func (noopGoogleConnectionStore) Put(_ context.Context, _ *googleConnectionRecord) error {
	return nil
}

func (store *dynamoGoogleConnectionStore) Delete(ctx context.Context, connectionID string) error {
	if strings.TrimSpace(connectionID) == "" {
		return nil
	}
	_, err := store.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		Key: map[string]dynamodbTypes.AttributeValue{
			"connection_id": &dynamodbTypes.AttributeValueMemberS{Value: connectionID},
		},
		TableName: &store.tableName,
	})
	return err
}

func (store *dynamoGoogleConnectionStore) Get(ctx context.Context, connectionID string) (*googleConnectionRecord, error) {
	if strings.TrimSpace(connectionID) == "" {
		return nil, nil
	}
	output, err := store.client.GetItem(ctx, &dynamodb.GetItemInput{
		Key: map[string]dynamodbTypes.AttributeValue{
			"connection_id": &dynamodbTypes.AttributeValueMemberS{Value: connectionID},
		},
		TableName: &store.tableName,
	})
	if err != nil || len(output.Item) == 0 {
		return nil, err
	}
	var record googleConnectionRecord
	if err := attributevalue.UnmarshalMap(output.Item, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func (store *dynamoGoogleConnectionStore) Put(ctx context.Context, record *googleConnectionRecord) error {
	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return err
	}
	_, err = store.client.PutItem(ctx, &dynamodb.PutItemInput{
		Item:      item,
		TableName: &store.tableName,
	})
	return err
}

const rateLimiterMaxKeys = 10000

func newLoginRateLimiter(maxAttempts int, window time.Duration) *loginRateLimiter {
	limiter := &loginRateLimiter{
		maxAttempts: maxAttempts,
		window:      window,
		attempts:    make(map[string]loginAttempt),
		stop:        make(chan struct{}),
	}
	go limiter.periodicCleanup()
	return limiter
}

func (limiter *loginRateLimiter) Close() {
	limiter.closeOnce.Do(func() { close(limiter.stop) })
}

func (limiter *loginRateLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}

	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	attempt := limiter.attempts[key]
	if now.Sub(attempt.WindowStart) > limiter.window {
		attempt = loginAttempt{WindowStart: now}
	}
	if attempt.WindowStart.IsZero() {
		attempt.WindowStart = now
	}
	if attempt.Count >= limiter.maxAttempts {
		return false
	}

	// Enforce upper bound on stored keys to prevent unbounded memory growth.
	// Sweep expired entries first so legitimate requests aren't blocked by stale keys.
	if _, exists := limiter.attempts[key]; !exists && len(limiter.attempts) >= rateLimiterMaxKeys {
		for candidate, a := range limiter.attempts {
			if now.Sub(a.WindowStart) > limiter.window {
				delete(limiter.attempts, candidate)
			}
			if len(limiter.attempts) < rateLimiterMaxKeys {
				break
			}
		}
		if len(limiter.attempts) >= rateLimiterMaxKeys {
			return false
		}
	}

	attempt.Count++
	limiter.attempts[key] = attempt
	return true
}

func (limiter *loginRateLimiter) periodicCleanup() {
	ticker := time.NewTicker(limiter.window)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			limiter.mu.Lock()
			now := time.Now()
			for key, attempt := range limiter.attempts {
				if now.Sub(attempt.WindowStart) > limiter.window {
					delete(limiter.attempts, key)
				}
			}
			limiter.mu.Unlock()
		case <-limiter.stop:
			return
		}
	}
}

func main() {
	mimeTypes := map[string]string{
		".css":  "text/css",
		".js":   "application/javascript",
		".ico":  "image/x-icon",
		".svg":  "image/svg+xml",
		".webp": "image/webp",
		".png":  "image/png",
		".jpg":  "image/jpeg",
	}
	for ext, mtype := range mimeTypes {
		if err := mime.AddExtensionType(ext, mtype); err != nil {
			log.Fatalf("Failed to add MIME type for %s: %v", ext, err)
		}
	}

	// routes - pages
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/about", aboutHandler)
	http.HandleFunc("/experience", experienceHandler)
	http.HandleFunc("/experience/timeline", experienceTimelineHandler)
	http.HandleFunc("/skills", skillsHandler)
	http.HandleFunc("/skills/grid", skillsGridHandler)
	http.HandleFunc("/skills/filtered", skillsFilteredHandler)
	http.HandleFunc("/skills/detail", skillsDetailHandler)
	http.HandleFunc("/projects", projectsHandler)
	http.HandleFunc("/projects/grid", projectsGridHandler)
	http.HandleFunc("/education", educationHandler)
	http.HandleFunc("/contact", contactHandler)

	// soccer routes
	http.HandleFunc("/soccer", soccerHandler)
	http.HandleFunc("/soccer/session", soccerSessionHandler)
	http.HandleFunc("/soccer/import", soccerImportHandler)
	http.HandleFunc("/soccer/logout", soccerLogoutHandler)
	http.HandleFunc("/soccer/google/add", soccerGoogleAddHandler)
	http.HandleFunc("/soccer/google/calendar", soccerGoogleCalendarHandler)
	http.HandleFunc("/soccer/google/connect", soccerGoogleConnectHandler)
	http.HandleFunc("/soccer/google/disconnect", soccerGoogleDisconnectHandler)
	http.HandleFunc("/soccer/fetch", fetchSchedulesHandler)
	http.HandleFunc("/soccer/download", downloadICSHandler)
	http.HandleFunc("/soccer/subscribe", subscribeHandler)

	// static files
	http.Handle(
		"/static/",
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("static")),
		),
	)

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/images/favicon.ico")
	})

	// Bind the listener before any slow init so health checks pass immediately.
	listenAddress := serverListenAddress()
	ln, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", listenAddress, err)
	}

	server := &http.Server{
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Craig Johnson Portfolio running at %s", localServerURL(listenAddress))
		log.Fatal(server.Serve(ln))
	}()

	// Initialize the Google connection store in the background so App Runner
	// health checks never wait on AWS SDK startup or credential resolution.
	if googleEnabled() {
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer initCancel()
			store, initErr := newGoogleConnectionStore(initCtx, &configData)
			if initErr != nil {
				log.Printf("google calendar add disabled: could not initialize connection store: %v", initErr)
				return
			}
			setGoogleConnectionStore(store)
		}()
	}

	select {}
}

/*
========================================
Home
========================================
*/

func gravatarURL(email string, size int) string {
	email = strings.TrimSpace(strings.ToLower(email))
	hash := md5.Sum([]byte(email))
	return "https://www.gravatar.com/avatar/" + hex.EncodeToString(hash[:]) + "?s=" + strconv.Itoa(size)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	err := pages.Home(pages.HomeProps{
		Name:               "Craig Johnson",
		Role:               "Cloud Engineer Principal",
		AvatarURL:          gravatarURL("gravatar@craigdevjohnson.com", 275),
		Description:        "Hi there! I'm a seasoned System Engineer with over a decade of experience in system engineering, administration, and optimization. I specialize in designing, implementing, and maintaining various systems and applications, thriving on performance optimization and security enhancement. I enjoy collaborating with application owners and software engineers to deliver innovative solutions and streamline processes through automation. I'm passionate about modernizing infrastructure and documenting critical processes. Let's connect and share our tech journeys!",
		YearsInTech:        time.Now().Year() - careerStartYear,
		Certifications:     10,
		AutomationProjects: "100",
	}).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
About
========================================
*/

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	props := pages.AboutProps{
		YearsInTech:    time.Now().Year() - careerStartYear,
		Certifications: 10,
		TechUsed:       30,
		CupsOfCoffee:   "∞",
	}
	err := pages.About(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Experience
========================================
*/

// Use types from shared package
type Experience = types.Experience

func experienceData() []Experience {
	return []Experience{
		{
			ID:               1,
			Position:         "Cloud Engineer Principal",
			Company:          "COMPANY REDACTED - A",
			Duration:         "2022 – Present",
			Responsibilities: "Lead infrastructure automation initiatives using IaC principles. Implement CI/CD pipelines for application deployment and configuration management. Architect and maintain cloud-native solutions while optimizing application performance and security. Develop self-service capabilities through automation, reducing deployment time by implementing GitOps methodologies.",
			Technologies:     []string{"AWS", "Go", "Terraform", "Ansible"},
			SkillAreas:       "cloud,automation,devops,scripting,security",
			Side:             "left",
		},
		{
			ID:               2,
			Position:         "System Administrator",
			Company:          "COMPANY REDACTED - B",
			Duration:         "2021 – 2022",
			Responsibilities: "Managed enterprise SCADA systems and infrastructure automation. Implemented monitoring solutions and maintained high-availability environments. Established IT/OT integration practices while ensuring regulatory compliance. Orchestrated application deployments and infrastructure upgrades in critical environments.",
			Technologies:     []string{"IoT", "SCADA", "RHEL", "Bash"},
			SkillAreas:       "systems,automation,security,scripting",
			Side:             "right",
		},
		{
			ID:               3,
			Position:         "IT Systems Engineer Sr",
			Company:          "COMPANY REDACTED - C",
			Duration:         "2020 – 2021",
			Responsibilities: "Architected and implemented cloud infrastructure solutions in healthcare environments. Led technical projects involving cross-functional teams and vendor integration. Developed automation frameworks for critical systems and established best practices for infrastructure management.",
			Technologies:     []string{"Azure", "AD DS", "PowerShell"},
			SkillAreas:       "cloud,systems,automation,scripting",
			Side:             "left",
		},
		{
			ID:               4,
			Position:         "IT Systems Engineer",
			Company:          "COMPANY REDACTED - C",
			Duration:         "2018 – 2020",
			Responsibilities: "Managed enterprise Active Directory and Exchange infrastructure. Implemented automation solutions for service deployment and configuration management. Orchestrated application lifecycle management and infrastructure upgrades.",
			Technologies:     []string{"PowerShell", "AD DS", "O365/Exchange"},
			SkillAreas:       "systems,automation,scripting",
			Side:             "right",
		},
		{
			ID:               5,
			Position:         "IT Desktop Engineer",
			Company:          "COMPANY REDACTED - C",
			Duration:         "2017 – 2018",
			Responsibilities: "Implemented automated solutions for endpoint management and configuration. Managed incident response for business-critical systems using ITIL methodologies. Established standardized deployment procedures for enterprise endpoints.",
			Technologies:     []string{"PowerShell", "SCCM", "Intune"},
			SkillAreas:       "systems,automation,scripting",
			Side:             "left",
		},
		{
			ID:               6,
			Position:         "IT Service Desk Associate",
			Company:          "COMPANY REDACTED - C",
			Duration:         "2016 – 2017",
			Responsibilities: "Utilized ITSM platforms for incident and change management. Maintained documentation for standard operating procedures. Provided technical support for enterprise applications and systems.",
			Technologies:     []string{"ServiceNow", "O365", "Windows"},
			SkillAreas:       "systems",
			Side:             "right",
		},
		{
			ID:               7,
			Position:         "Service Desk Student Analyst",
			Company:          "COMPANY REDACTED - D",
			Duration:         "2012 – 2016",
			Responsibilities: "Managed incident tracking through enterprise ITSM systems. Maintained technical documentation and knowledge base articles. Achieved consistent high-quality metrics in service delivery.",
			Technologies:     []string{"Windows", "MacOS", "GoogleApps"},
			SkillAreas:       "systems",
			Side:             "left",
		},
	}
}

func experienceHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Experience().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func experienceTimelineHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.ExperienceTimelineProps{
		Experiences: experienceData(),
	}
	err := partials.ExperienceTimeline(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Skills
========================================
*/

// Use types from shared package
type (
	Skill         = types.Skill
	SkillCategory = types.SkillCategory
)

const (
	iconZeroTrust      string = `<svg viewBox="0 0 24 24" fill="#8B5CF6" aria-hidden="true"><path d="M12 1l9 4v6c0 5.25-3.81 10.14-9 11-5.19-.86-9-5.75-9-11V5l9-4zm0 2.18L5 6.3v4.7c0 4.08 2.96 7.88 7 8.62 4.04-.74 7-4.54 7-8.62V6.3l-7-3.12zM12 7a2 2 0 110 4 2 2 0 010-4zm0 5c2.67 0 8 1.34 8 4v1H4v-1c0-2.66 5.33-4 8-4z"/></svg>`
	iconIdentityAccess string = `<svg viewBox="0 0 24 24" fill="#F59E0B" aria-hidden="true"><path d="M18.685 19.097A9.723 9.723 0 0021.75 12c0-5.385-4.365-9.75-9.75-9.75S2.25 6.615 2.25 12a9.723 9.723 0 003.065 7.097A9.716 9.716 0 0012 21.75a9.716 9.716 0 006.685-2.653zm-12.54-1.285A7.486 7.486 0 0112 15a7.486 7.486 0 015.855 2.812A8.224 8.224 0 0112 20.25a8.224 8.224 0 01-5.855-2.438zM15.75 9a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z"/></svg>`
	iconCloudSecurity  string = `<svg viewBox="0 0 24 24" fill="#EF4444" aria-hidden="true"><path d="M4.5 9.75a6 6 0 0111.573-2.226 3.75 3.75 0 014.133 4.303A4.5 4.5 0 0118 20.25H6.75a5.25 5.25 0 01-2.23-10.004 6.072 6.072 0 01-.02-.496z"/><path fill="#fff" d="M12 8l3 3h-2v3h-2v-3H9l3-3z"/></svg>`
	iconCompliance     string = `<svg viewBox="0 0 24 24" fill="#22C55E" aria-hidden="true"><path d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`
	iconMonitoring     string = `<svg viewBox="0 0 24 24" fill="#06B6D4" aria-hidden="true"><path d="M3 13h2v8H3v-8zm6-6h2v14H9V7zm6-4h2v18h-2V3zm6 8h2v10h-2V11z"/></svg>`
	iconInfraAuto      string = `<svg viewBox="0 0 24 24" fill="#A855F7" aria-hidden="true"><path d="M4 6h16v2H4V6zm0 5h16v2H4v-2zm0 5h16v2H4v-2z"/><path d="M18 9l3 3-3 3M6 9l-3 3 3 3" stroke="#A855F7" stroke-width="1.5" fill="none"/></svg>`
	iconCloudArch      string = `<svg viewBox="0 0 24 24" fill="#0EA5E9" aria-hidden="true"><path d="M4.5 9.75a6 6 0 0111.573-2.226 3.75 3.75 0 014.133 4.303A4.5 4.5 0 0118 20.25H6.75a5.25 5.25 0 01-2.23-10.004 6.072 6.072 0 01-.02-.496z"/></svg>`
	iconNetworkSec     string = `<svg viewBox="0 0 24 24" fill="#EC4899" aria-hidden="true"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>`
	iconDevSecOps      string = `<svg viewBox="0 0 24 24" fill="#10B981" aria-hidden="true"><path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"/></svg>`
	iconSRE            string = `<svg viewBox="0 0 24 24" fill="#F97316" aria-hidden="true"><path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83"/><circle cx="12" cy="12" r="4" fill="#F97316"/></svg>`
	iconSecOps         string = `<svg viewBox="0 0 24 24" fill="#6366F1" aria-hidden="true"><path d="M12 2.25c-5.385 0-9.75 4.365-9.75 9.75s4.365 9.75 9.75 9.75 9.75-4.365 9.75-9.75S17.385 2.25 12 2.25zM12.75 6a.75.75 0 00-1.5 0v6c0 .414.336.75.75.75h4.5a.75.75 0 000-1.5h-3.75V6z"/></svg>`
)

func skillsData() []SkillCategory {
	return []SkillCategory{
		{
			Name: "Languages & Scripting",
			Skills: []Skill{
				{ID: 5, Name: "Bash", IconPath: "/static/images/skills/bash.svg", Link: "https://www.gnu.org/software/bash/", Proficiency: "expert", Featured: true, Description: "Unix shell and command language for task automation and system administration"},
				{ID: 2, Name: "Go", IconPath: "/static/images/skills/go.svg", Link: "https://go.dev/", Proficiency: "advanced", Featured: true, Description: "Statically typed language for building scalable cloud services and CLI tools"},
				{ID: 3, Name: "JavaScript", IconPath: "/static/images/skills/javascript.svg", Link: "https://developer.mozilla.org/en-US/docs/Web/JavaScript", Proficiency: "advanced"},
				{ID: 10, Name: "JSON", IconPath: "/static/images/skills/json.svg", Link: "https://www.json.org/", Proficiency: "expert"},
				{ID: 11, Name: "Markdown", IconPath: "/static/images/skills/markdown.svg", Link: "https://www.markdownguide.org/", Proficiency: "expert"},
				{ID: 6, Name: "PowerShell", IconPath: "/static/images/skills/powershell.svg", Link: "https://learn.microsoft.com/en-us/powershell/", Proficiency: "expert", Featured: true, Description: "Cross-platform framework for configuration management and task automation"},
				{ID: 1, Name: "Python", IconPath: "/static/images/skills/python.svg", Link: "https://www.python.org/", Proficiency: "expert", Featured: true, Description: "Versatile language for automation, scripting, and cloud infrastructure tooling"},
				{ID: 4, Name: "TypeScript", IconPath: "/static/images/skills/typescript.svg", Link: "https://www.typescriptlang.org/", Proficiency: "intermediate"},
				{ID: 9, Name: "YAML", IconPath: "/static/images/skills/yaml.svg", Link: "https://yaml.org/", Proficiency: "expert"},
			},
		},
		{
			Name: "Cloud Platforms",
			Skills: []Skill{
				{ID: 12, Name: "AWS", IconPath: "/static/images/skills/aws.svg", Link: "https://aws.amazon.com/", Proficiency: "expert", Featured: true, Description: "Primary cloud platform for compute, storage, networking, and serverless solutions"},
				{ID: 13, Name: "Azure", IconPath: "/static/images/skills/azure.svg", Link: "https://azure.microsoft.com/", Proficiency: "advanced", Featured: true, Description: "Microsoft cloud platform for hybrid identity, VMs, and enterprise services"},
				{ID: 15, Name: "Cloudflare", IconPath: "/static/images/skills/cloudflare.svg", Link: "https://www.cloudflare.com/", Proficiency: "intermediate"},
				{ID: 17, Name: "vSphere", IconPath: "/static/images/skills/vsphere.svg", Link: "https://www.vmware.com/products/vsphere.html", Proficiency: "advanced"},
			},
		},
		{
			Name: "Security & Identity",
			Skills: []Skill{
				{ID: 121, Name: "Cognito", IconPath: "/static/images/skills/aws_cognito.svg", Link: "https://aws.amazon.com/cognito/", Proficiency: "advanced"},
				{ID: 120, Name: "IAM", IconPath: "/static/images/skills/aws_iam.svg", Link: "https://aws.amazon.com/iam/", Proficiency: "expert", Featured: true, Description: "Identity and access management for implementing least-privilege security"},
				{ID: 30, Name: "Vault", IconPath: "/static/images/skills/hashicorp_vault.svg", Link: "https://www.vaultproject.io/", Proficiency: "advanced"},
			},
		},
		{
			Name: "Containers & Orchestration",
			Skills: []Skill{
				{ID: 18, Name: "Docker", IconPath: "/static/images/skills/docker.svg", Link: "https://www.docker.com/", Proficiency: "expert", Featured: true, Description: "Container platform for building, shipping, and running applications consistently"},
				{ID: 19, Name: "Kubernetes", IconPath: "/static/images/skills/kubernetes.svg", Link: "https://kubernetes.io/", Proficiency: "advanced", Featured: true, Description: "Container orchestration for deploying and scaling containerized workloads"},
				{ID: 20, Name: "Podman", IconPath: "/static/images/skills/podman.svg", Link: "https://podman.io/", Proficiency: "advanced", Featured: true, Description: "Daemonless container engine for running OCI containers and pods"},
				{ID: 101, Name: "Rancher", IconPath: "/static/images/skills/rancher.svg", Link: "https://www.rancher.com/", Proficiency: "intermediate"},
			},
		},
		{
			Name: "CI/CD & Automation",
			Skills: []Skill{
				{ID: 27, Name: "Ansible", IconPath: "/static/images/skills/ansible.svg", Link: "https://www.ansible.com/", Proficiency: "expert", Featured: true, Description: "Agentless automation for configuration management and application deployment"},
				{ID: 125, Name: "CodeBuild", IconPath: "/static/images/skills/aws_codebuild.svg", Link: "https://aws.amazon.com/codebuild/", Proficiency: "advanced"},
				{ID: 126, Name: "CodeDeploy", IconPath: "/static/images/skills/aws_codedeploy.svg", Link: "https://aws.amazon.com/codedeploy/", Proficiency: "advanced"},
				{ID: 127, Name: "CodePipeline", IconPath: "/static/images/skills/aws_codepipeline.svg", Link: "https://aws.amazon.com/codepipeline/", Proficiency: "advanced"},
				{ID: 22, Name: "GitHub Actions", IconPath: "/static/images/skills/github_actions.svg", Link: "https://github.com/features/actions", Proficiency: "expert", Featured: true, Description: "CI/CD platform for automating build, test, and deployment workflows"},
				{ID: 24, Name: "Jenkins", IconPath: "/static/images/skills/jenkins.svg", Link: "https://www.jenkins.io/", Proficiency: "advanced"},
				{ID: 28, Name: "Packer", IconPath: "/static/images/skills/packer.svg", Link: "https://www.packer.io/", Proficiency: "intermediate"},
				{ID: 103, Name: "Puppet", IconPath: "/static/images/skills/puppet.svg", Link: "https://www.puppet.com/", Proficiency: "intermediate"},
			},
		},
		{
			Name: "Infrastructure as Code",
			Skills: []Skill{
				{ID: 107, Name: "CloudFormation", IconPath: "/static/images/skills/cloudformation.svg", Link: "https://aws.amazon.com/cloudformation/", Proficiency: "expert", Featured: true, Description: "AWS-native infrastructure as code for provisioning cloud resources"},
				{ID: 104, Name: "OpenTofu", IconPath: "/static/images/skills/opentofu.svg", Link: "https://opentofu.org/", Proficiency: "advanced"},
				{ID: 29, Name: "Terraform", IconPath: "/static/images/skills/hashicorp_terraform.svg", Link: "https://www.terraform.io/", Proficiency: "expert", Featured: true, Description: "Multi-cloud infrastructure as code for declarative resource provisioning"},
				{ID: 105, Name: "Terragrunt", IconPath: "/static/images/skills/terragrunt.svg", Link: "https://terragrunt.gruntwork.io/", Proficiency: "advanced"},
				{ID: 106, Name: "Terramate", IconPath: "/static/images/skills/terramate.svg", Link: "https://terramate.io/", Proficiency: "intermediate"},
			},
		},
		{
			Name: "Databases",
			Skills: []Skill{
				{ID: 36, Name: "DynamoDB", IconPath: "/static/images/skills/dynamodb.svg", Link: "https://aws.amazon.com/dynamodb/", Proficiency: "advanced"},
				{ID: 38, Name: "Elasticsearch", IconPath: "/static/images/skills/elasticsearch.svg", Link: "https://www.elastic.co/elasticsearch/", Proficiency: "intermediate"},
				{ID: 34, Name: "MongoDB", IconPath: "/static/images/skills/mongodb.svg", Link: "https://www.mongodb.com/", Proficiency: "intermediate"},
				{ID: 32, Name: "MySQL", IconPath: "/static/images/skills/mysql.svg", Link: "https://www.mysql.com/", Proficiency: "advanced"},
				{ID: 31, Name: "PostgreSQL", IconPath: "/static/images/skills/postgresql.svg", Link: "https://www.postgresql.org/", Proficiency: "advanced"},
				{ID: 35, Name: "Redis", IconPath: "/static/images/skills/redis.svg", Link: "https://redis.io/", Proficiency: "intermediate"},
				{ID: 33, Name: "SQL Server", IconPath: "/static/images/skills/microsoft_sql_server.svg", Link: "https://www.microsoft.com/en-us/sql-server", Proficiency: "advanced"},
				{ID: 37, Name: "SQLite", IconPath: "/static/images/skills/sqlite.svg", Link: "https://www.sqlite.org/", Proficiency: "intermediate"},
			},
		},
		{
			Name: "API & Testing",
			Skills: []Skill{
				{ID: 124, Name: "API Gateway", IconPath: "/static/images/skills/aws_api_gateway.svg", Link: "https://aws.amazon.com/api-gateway/", Proficiency: "advanced"},
				{ID: 39, Name: "FastAPI", IconPath: "/static/images/skills/fastapi.svg", Link: "https://fastapi.tiangolo.com/", Proficiency: "intermediate"},
				{ID: 40, Name: "OpenAPI", IconPath: "/static/images/skills/openapi.svg", Link: "https://www.openapis.org/", Proficiency: "advanced"},
				{ID: 43, Name: "Playwright", IconPath: "/static/images/skills/playwright.svg", Link: "https://playwright.dev/", Proficiency: "advanced"},
				{ID: 41, Name: "Postman", IconPath: "/static/images/skills/postman.svg", Link: "https://www.postman.com/", Proficiency: "advanced"},
				{ID: 42, Name: "pytest", IconPath: "/static/images/skills/pytest.svg", Link: "https://docs.pytest.org/", Proficiency: "advanced"},
			},
		},
		{
			Name: "Development Tools",
			Skills: []Skill{
				{ID: 44, Name: "Git", IconPath: "/static/images/skills/git.svg", Link: "https://git-scm.com/", Proficiency: "expert", Featured: true, Description: "Distributed version control for collaborative development and code management"},
				{ID: 45, Name: "GitHub", IconPath: "/static/images/skills/github.svg", Link: "https://github.com/", Proficiency: "expert"},
				{ID: 46, Name: "GitHub Codespaces", IconPath: "/static/images/skills/github_codespaces.svg", Link: "https://github.com/features/codespaces", Proficiency: "advanced"},
				{ID: 50, Name: "Node.js", IconPath: "/static/images/skills/node.js.svg", Link: "https://nodejs.org/", Proficiency: "advanced"},
				{ID: 49, Name: "npm", IconPath: "/static/images/skills/npm.svg", Link: "https://www.npmjs.com/", Proficiency: "advanced"},
				{ID: 51, Name: "Poetry", IconPath: "/static/images/skills/python_poetry.svg", Link: "https://python-poetry.org/", Proficiency: "advanced"},
				{ID: 52, Name: "Vite", IconPath: "/static/images/skills/vite.js.svg", Link: "https://vitejs.dev/", Proficiency: "intermediate"},
				{ID: 47, Name: "VS Code", IconPath: "/static/images/skills/vscode.svg", Link: "https://code.visualstudio.com/", Proficiency: "expert"},
			},
		},
		{
			Name: "Monitoring & Observability",
			Skills: []Skill{
				{ID: 108, Name: "CloudWatch", IconPath: "/static/images/skills/cloudwatch.svg", Link: "https://aws.amazon.com/cloudwatch/", Proficiency: "expert"},
				{ID: 55, Name: "Datadog", IconPath: "/static/images/skills/datadog.svg", Link: "https://www.datadoghq.com/", Proficiency: "advanced"},
				{ID: 54, Name: "Grafana", IconPath: "/static/images/skills/grafana.svg", Link: "https://grafana.com/", Proficiency: "intermediate"},
				{ID: 53, Name: "Prometheus", IconPath: "/static/images/skills/prometheus.svg", Link: "https://prometheus.io/", Proficiency: "intermediate"},
				{ID: 56, Name: "Splunk", IconPath: "/static/images/skills/splunk.svg", Link: "https://www.splunk.com/", Proficiency: "advanced"},
			},
		},
		{
			Name: "Operating Systems",
			Skills: []Skill{
				{ID: 111, Name: "Debian", IconPath: "/static/images/skills/debian.svg", Link: "https://www.debian.org/", Proficiency: "advanced"},
				{ID: 60, Name: "Raspberry Pi", IconPath: "/static/images/skills/raspberrypi.svg", Link: "https://www.raspberrypi.org/", Proficiency: "intermediate"},
				{ID: 109, Name: "RHEL", IconPath: "/static/images/skills/red_hat.svg", Link: "https://www.redhat.com/en/technologies/linux-platforms/enterprise-linux", Proficiency: "expert"},
				{ID: 110, Name: "Ubuntu", IconPath: "/static/images/skills/ubuntu.svg", Link: "https://ubuntu.com/", Proficiency: "expert"},
				{ID: 59, Name: "Windows", IconPath: "/static/images/skills/windows.svg", Link: "https://www.microsoft.com/windows/", Proficiency: "expert"},
				{ID: 57, Name: "Linux", IconPath: "/static/images/skills/linux.svg", Link: "https://www.linux.org/", Proficiency: "expert", Featured: true, Description: "Primary operating system for servers, containers, and cloud infrastructure"},
			},
		},
		{
			Name: "Web Servers & Frameworks",
			Skills: []Skill{
				{ID: 62, Name: "Apache", IconPath: "/static/images/skills/apache.svg", Link: "https://httpd.apache.org/", Proficiency: "advanced"},
				{ID: 61, Name: "Nginx", IconPath: "/static/images/skills/nginx.svg", Link: "https://nginx.org/", Proficiency: "advanced"},
				{ID: 123, Name: "Amplify", IconPath: "/static/images/skills/aws_amplify.svg", Link: "https://aws.amazon.com/amplify/", Proficiency: "advanced"},
				{ID: 64, Name: "Vue.js", IconPath: "/static/images/skills/vue.js.svg", Link: "https://vuejs.org/", Proficiency: "advanced"},
			},
		},
		{
			Name: "Collaboration Tools",
			Skills: []Skill{
				{ID: 67, Name: "Confluence", IconPath: "/static/images/skills/confluence.svg", Link: "https://www.atlassian.com/software/confluence", Proficiency: "advanced"},
				{ID: 66, Name: "Jira", IconPath: "/static/images/skills/jira.svg", Link: "https://www.atlassian.com/software/jira", Proficiency: "advanced"},
				{ID: 119, Name: "Notion", IconPath: "/static/images/skills/notion.svg", Link: "https://www.notion.so/", Proficiency: "intermediate"},
				{ID: 68, Name: "Slack", IconPath: "/static/images/skills/slack.svg", Link: "https://slack.com/", Proficiency: "expert"},
			},
		},
		{
			Name: "Concepts & Practices",
			Skills: []Skill{
				{ID: 75, Name: "Cloud Architecture", Icon: iconCloudArch, Link: "https://aws.amazon.com/architecture/", Proficiency: "expert"},
				{ID: 71, Name: "Cloud Security", Icon: iconCloudSecurity, Link: "https://www.checkpoint.com/cyber-hub/cloud-security/what-is-cloud-security/", Proficiency: "expert"},
				{ID: 72, Name: "Compliance & Governance", Icon: iconCompliance, Link: "https://www.rapid7.com/fundamentals/compliance-regulatory-frameworks/", Proficiency: "advanced"},
				{ID: 77, Name: "DevSecOps", Icon: iconDevSecOps, Link: "https://www.redhat.com/en/topics/devops/what-is-devsecops", Proficiency: "expert"},
				{ID: 70, Name: "Identity & Access Management", Icon: iconIdentityAccess, Link: "https://www.gartner.com/en/information-technology/glossary/identity-and-access-management-iam", Proficiency: "expert"},
				{ID: 74, Name: "Infrastructure Automation", Icon: iconInfraAuto, Link: "https://www.redhat.com/en/topics/automation/what-is-infrastructure-as-code-iac", Proficiency: "expert"},
				{ID: 76, Name: "Network Security", Icon: iconNetworkSec, Link: "https://www.cisco.com/c/en/us/products/security/what-is-network-security.html", Proficiency: "advanced"},
				{ID: 73, Name: "Observability", Icon: iconMonitoring, Link: "https://newrelic.com/blog/best-practices/what-is-observability", Proficiency: "advanced"},
				{ID: 79, Name: "Security Operations", Icon: iconSecOps, Link: "https://www.microsoft.com/en-us/security/business/security-101/what-is-a-security-operations-center-soc", Proficiency: "advanced"},
				{ID: 78, Name: "Site Reliability Engineering", Icon: iconSRE, Link: "https://sre.google/", Proficiency: "advanced"},
				{ID: 69, Name: "Zero Trust Architecture", Icon: iconZeroTrust, Link: "https://www.cloudflare.com/learning/security/glossary/what-is-zero-trust/", Proficiency: "advanced"},
			},
		},
	}
}

// getFeaturedSkills extracts all featured skills from provided categories
func getFeaturedSkills(categories []SkillCategory) []Skill {
	var featured []Skill
	for _, category := range categories {
		for i := range category.Skills {
			if category.Skills[i].Featured {
				category.Skills[i].Category = category.Name
				featured = append(featured, category.Skills[i])
			}
		}
	}
	return featured
}

func skillsHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Skills().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func skillsGridHandler(w http.ResponseWriter, r *http.Request) {
	categories := skillsData()
	props := partials.SkillsGridProps{
		Categories:     categories,
		FeaturedSkills: getFeaturedSkills(categories),
	}
	err := partials.SkillsGrid(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func skillsFilteredHandler(w http.ResponseWriter, r *http.Request) {
	categories := skillsData()
	activeCategory := r.URL.Query().Get("category")
	activeProficiency := r.URL.Query().Get("proficiency")

	props := partials.SkillsFilterableProps{
		Categories:        categories,
		ActiveCategory:    activeCategory,
		ActiveProficiency: activeProficiency,
	}
	err := partials.SkillsFilterableSection(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func skillsDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	categories := skillsData()
	var found Skill
	var foundCategory string
	for _, cat := range categories {
		for i := range cat.Skills {
			if cat.Skills[i].ID == id {
				found = cat.Skills[i]
				foundCategory = cat.Name
				break
			}
		}
		if found.Name != "" {
			break
		}
	}

	if found.Name == "" {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	found.Category = foundCategory
	props := partials.SkillDetailProps{
		Skill: found,
	}
	err = partials.SkillDetail(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Projects
========================================
*/

// Use types from shared package
type Project = types.Project

func projectsData() []Project {
	return []Project{
		{
			ID:           1,
			Name:         "Personal Portfolio Website",
			Intro:        "A modern, responsive portfolio built with Go and HTMX",
			Description:  "Showcases my projects, skills, and certifications with a focus on cloud and web technologies.",
			Technologies: []string{"Go", "HTMX", "CSS", "HTML", "GitHub", "AWS"},
			Image:        "/static/images/projects/portfolio.webp",
			GitHubURL:    "https://github.com/CraigDevJohnson/craig-johnson-portfolio-vue",
			DemoURL:      "https://craigdevjohnson.com",
			Category:     "Web",
		},
		{
			ID:           2,
			Name:         "New User Account Provisioning",
			Intro:        "PowerShell scripts to fully automate user account creation and configuration.",
			Description:  "Completely automated new user account creation and configuration based on database push of new user information. This automation included creating the new user's active directory account, email account in O365/Exchange, and role based group memberships.",
			Technologies: []string{"PowerShell", "Git", "APIs", "AD DS", "O365/Exchange"},
			Image:        "/static/images/projects/provisioning.webp",
			Category:     "Automation",
		},
		{
			ID:           3,
			Name:         "Soccer Schedule Scraper",
			Intro:        "A web scraper to pull and parse team schedules and download as ICS file.",
			Description:  "A multi function Python script deployed as an AWS Lambda function to scrape and parse soccer team schedules and return them in ICS file format for broadly supported calendar importing.",
			Technologies: []string{"Python", "AWS Lambda", "GitHub", "API"},
			Image:        "/static/images/projects/scraper.webp",
			GitHubURL:    "https://github.com/CraigDevJohnson/soccer-scraper",
			DemoURL:      "/soccer",
			Category:     "Automation",
		},
	}
}

func projectsHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Projects().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func projectsGridHandler(w http.ResponseWriter, r *http.Request) {
	props := partials.ProjectsGridProps{
		Projects: projectsData(),
	}
	err := partials.ProjectsGrid(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Education
========================================
*/

func educationHandler(w http.ResponseWriter, r *http.Request) {
	props := pages.EducationProps{
		TotalCerts:      10,
		Providers:       5,
		YearsCertifying: time.Now().Year() - 2018,
	}
	if err := pages.Education(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Contact
========================================
*/

func contactHandler(w http.ResponseWriter, r *http.Request) {
	err := pages.Contact().Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/*
========================================
Soccer
========================================
*/

// Use types from shared package
type (
	Game                 = types.Game
	GoogleCalendarOption = types.GoogleCalendarOption
	LambdaGamesResponse  = types.LambdaGamesResponse
	LPSPlayer            = types.LPSPlayer
	SessionData          = types.SessionData
)

func soccerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("code") != "" || r.URL.Query().Get("error") != "" || r.URL.Query().Get("state") != "" {
		soccerGoogleCallbackHandler(w, r)
		return
	}
	props := pages.SoccerProps{
		GoogleMessage:     soccerGoogleFlashMessage(r.URL.Query().Get("google")),
		GoogleMessageKind: soccerGoogleFlashKind(r.URL.Query().Get("google")),
	}
	err := pages.Soccer(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func soccerSessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, _ := loadSoccerSession(w, r)

	renderSoccerLoginState(w, r, session)
}

func soccerImportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !loginEnabled() {
		renderSoccerLoginFeedback(w, "error", "JWT import is unavailable until the session encryption key is configured on the server.")
		return
	}
	if !soccerLoginAttempts.Allow(clientIP(r)) {
		renderSoccerLoginFeedback(w, "error", "Too many import attempts. Wait a minute and try again.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		renderSoccerLoginFeedback(w, "error", "Could not read the import form. Try again.")
		return
	}

	jwt, err := normalizeImportedJWT(r.FormValue("jwt"))
	if err != nil {
		renderSoccerLoginFeedback(w, "error", err.Error())
		return
	}

	discovery, err := lpsFetchUserPlayers(r.Context(), jwt)
	if err != nil {
		var fetchErr *lpsFetchError
		if errors.As(err, &fetchErr) {
			switch fetchErr.Kind {
			case lpsErrorUnauthorized, lpsErrorForbidden:
				renderSoccerLoginFeedback(w, "error", "The JWT was rejected by Let's Play Soccer. Copy a fresh bearer token and try again.")
				return
			case lpsErrorUpstream:
				renderSoccerLoginFeedback(w, "error", "Could not reach Let's Play Soccer to look up your players. Try again in a moment.")
				return
			}
		}
		renderSoccerLoginFeedback(w, "error", err.Error())
		return
	}
	if len(discovery.Players) == 0 {
		renderSoccerLoginFeedback(w, "error", "No linked players found for this account.")
		return
	}

	session := SessionData{
		JWT:       jwt,
		UserName:  discovery.UserName,
		Players:   discovery.Players,
		ExpiresAt: importedSessionExpiry(jwt),
	}
	if err := setSession(w, r, &session); err != nil {
		log.Printf("soccer import session write failed: %v", err)
		renderSoccerLoginFeedback(w, "error", "The import succeeded, but the session cookie could not be saved.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SoccerLoginState(soccerLoginStateProps(w, r, &session, true)).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, `<div class="soccer-login-success" data-login-success>Import saved for this browser session. Choose your players below.</div>`)
}

func soccerLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clearSession(w, r)
	w.Header().Set("HX-Trigger", "soccer-logout")
	renderSoccerLoginState(w, r, nil)
}

func redirectSoccerWithGoogleStatus(w http.ResponseWriter, r *http.Request, status string) {
	target := "/soccer"
	if strings.TrimSpace(status) != "" {
		target += "?google=" + url.QueryEscape(status)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func soccerGoogleConnectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !googleEnabled() {
		redirectSoccerWithGoogleStatus(w, r, "unavailable")
		return
	}
	connectionID := getGoogleConnectionID(r)
	if connectionID == "" {
		var err error
		connectionID, err = newRandomHex(16)
		if err != nil {
			log.Printf("google connection id generation failed: %v", err)
			redirectSoccerWithGoogleStatus(w, r, "failed")
			return
		}
	}
	state, err := newGoogleOAuthState(connectionID)
	if err != nil {
		log.Printf("google oauth state generation failed: %v", err)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	if err := setGoogleOAuthStateCookie(w, r, state); err != nil {
		log.Printf("google oauth state cookie write failed: %v", err)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	authURL := googleOAuthConfigForRequest(r).AuthCodeURL(
		state.State,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

func soccerGoogleCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if !googleEnabled() {
		redirectSoccerWithGoogleStatus(w, r, "unavailable")
		return
	}
	if strings.TrimSpace(r.URL.Query().Get("error")) != "" {
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "denied")
		return
	}
	state, err := getGoogleOAuthStateCookie(r)
	if errors.Is(err, errSessionExpired) {
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	if err != nil || state == nil || state.State == "" || state.State != strings.TrimSpace(r.URL.Query().Get("state")) {
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	ctx := googleHTTPContext(r.Context())
	token, err := googleOAuthConfigForRequest(r).Exchange(ctx, strings.TrimSpace(r.URL.Query().Get("code")))
	if err != nil {
		log.Printf("google token exchange failed: %v", err)
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	calendars, err := googleListCalendarsWithToken(ctx, token)
	if err != nil || len(calendars) == 0 {
		log.Printf("google calendar list after connect failed: %v", err)
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	selectedCalendarID, selectedCalendarSummary := preferredGoogleCalendar(calendars)
	encryptedToken, err := encryptGoogleToken(token)
	if err != nil {
		log.Printf("google token encryption failed: %v", err)
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	createdAt := time.Now().UTC()
	if existing, err := currentGoogleConnectionStore().Get(r.Context(), state.ConnectionID); err == nil && existing != nil && !existing.CreatedAt.IsZero() {
		createdAt = existing.CreatedAt
	}
	record := googleConnectionRecord{
		ConnectionID:    state.ConnectionID,
		TokenCiphertext: encryptedToken,
		CalendarID:      selectedCalendarID,
		CalendarSummary: selectedCalendarSummary,
		CreatedAt:       createdAt,
		UpdatedAt:       time.Now().UTC(),
	}
	if err := currentGoogleConnectionStore().Put(r.Context(), &record); err != nil {
		log.Printf("google connection save failed: %v", err)
		clearGoogleOAuthStateCookie(w, r)
		redirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	setGoogleConnectionCookie(w, r, state.ConnectionID)
	clearGoogleOAuthStateCookie(w, r)
	redirectSoccerWithGoogleStatus(w, r, "connected")
}

func soccerGoogleCalendarHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, _ := loadSoccerSession(w, r)
	if !googleEnabled() {
		renderSoccerLoginState(w, r, session)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		renderSoccerLoginState(w, r, session)
		return
	}
	record, err := loadGoogleConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
		renderSoccerLoginState(w, r, session)
		return
	}
	calendars, err := googleListCalendars(r.Context(), r, record)
	if err != nil {
		log.Printf("google calendar list failed: %v", err)
		renderSoccerLoginState(w, r, session)
		return
	}
	selectedCalendarID := strings.TrimSpace(r.FormValue("calendar_id"))
	selectedCalendarSummary := googleCalendarSummary(calendars, selectedCalendarID)
	if selectedCalendarSummary == "" {
		selectedCalendarID, selectedCalendarSummary = preferredGoogleCalendar(calendars)
	}
	record.CalendarID = selectedCalendarID
	record.CalendarSummary = selectedCalendarSummary
	record.UpdatedAt = time.Now().UTC()
	if err := currentGoogleConnectionStore().Put(r.Context(), record); err != nil {
		log.Printf("google calendar selection save failed: %v", err)
	}
	renderSoccerLoginState(w, r, session)
}

func soccerGoogleDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, _ := loadSoccerSession(w, r)
	deleteGoogleConnection(r.Context(), w, r)
	renderSoccerLoginState(w, r, session)
}

func renderGoogleDisconnectFeedback(w http.ResponseWriter, r *http.Request, session *SessionData, message string) {
	deleteGoogleConnection(r.Context(), w, r)
	if session != nil {
		if err := partials.SoccerLoginState(soccerLoginStateProps(w, r, session, true)).Render(context.Background(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	renderSoccerLoginFeedback(w, "error", message)
}

func isGoogleAuthRejection(resp *http.Response) bool {
	apiErr := readGoogleAPIError(resp)
	log.Printf("google event insert rejected: %v", apiErr)
	var googleErr *googleAPIError
	return errors.As(apiErr, &googleErr) && (googleErr.StatusCode == http.StatusUnauthorized || googleErr.StatusCode == http.StatusForbidden)
}

func soccerGoogleAddHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !googleEnabled() {
		renderSoccerLoginFeedback(w, "error", "Google Calendar add is unavailable until Google OAuth and server-side storage are configured.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		renderSoccerLoginFeedback(w, "error", "Could not read the selected games. Try again.")
		return
	}
	record, err := loadGoogleConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
		renderSoccerLoginFeedback(w, "error", "Connect Google Calendar before adding selected games.")
		return
	}
	selectedIDs := parseSelectedIDs(r.Form)
	if len(selectedIDs) == 0 {
		renderSoccerLoginFeedback(w, "error", "Select at least one game to add to Google Calendar.")
		return
	}
	teamCodes := r.FormValue("team_codes")
	rawPlayerIDs := r.Form["player_ids"]
	playerIDs := parsePlayerIDs(rawPlayerIDs)
	if len(nonEmptyStrings(rawPlayerIDs)) > 0 && len(playerIDs) == 0 {
		renderSoccerLoginFeedback(w, "error", "One or more selected players were invalid. Clear the imported players and import again to refresh the discovered list.")
		return
	}
	session, _ := loadSoccerSession(w, r)
	games, err := requestedScheduleGames(r.Context(), session, playerIDs, teamCodes)
	if err != nil {
		renderSoccerLoginFeedback(w, "error", googleAddScheduleErrorMessage(err))
		return
	}
	filteredGames := selectedScheduleGames(games, selectedIDs)
	if len(filteredGames) == 0 {
		renderSoccerLoginFeedback(w, "error", "No selected games were found to add.")
		return
	}
	token, err := currentGoogleToken(r.Context(), r, record)
	if err != nil {
		log.Printf("google token refresh failed: %v", err)
		renderGoogleDisconnectFeedback(w, r, session, "Your Google Calendar connection has expired. Connect again and retry.")
		return
	}
	added, existing, authRejected, err := insertGoogleCalendarEvents(r, record, token, filteredGames)
	if err != nil {
		log.Printf("google event insert failed: %v", err)
		renderSoccerLoginFeedback(w, "error", "Could not add the selected games to Google Calendar. Try again.")
		return
	}
	if authRejected {
		renderGoogleDisconnectFeedback(w, r, session, "Your Google Calendar connection is no longer valid. Connect again and retry.")
		return
	}
	message := fmt.Sprintf("Added %d selected game(s) to Google Calendar.", added)
	if existing > 0 {
		message += fmt.Sprintf(" Skipped %d game(s) that were already present.", existing)
	}
	renderSoccerLoginFeedback(w, "success", message)
}

func requestedScheduleGames(ctx context.Context, session *SessionData, playerIDs []int, teamCodes string) ([]Game, error) {
	teamIDs := parseTeamIDs(teamCodes)
	switch {
	case session != nil && len(playerIDs) > 0:
		return resolveScheduleGames(ctx, session, playerIDs, nil)
	case len(playerIDs) > 0:
		return nil, errPlayerSessionRequired
	case strings.TrimSpace(teamCodes) != "" && len(teamIDs) == 0:
		return nil, errInvalidTeamSelection
	case len(teamIDs) > 0:
		return resolveScheduleGames(ctx, session, nil, teamIDs)
	default:
		return nil, errScheduleSelection
	}
}

func googleAddScheduleErrorMessage(err error) string {
	switch {
	case errors.Is(err, errPlayerSessionRequired):
		return "Import a bearer JWT again before adding schedules for discovered players to Google Calendar."
	case errors.Is(err, errInvalidTeamSelection):
		return "Enter one or more numeric Let's Play Soccer team IDs separated by commas."
	case errors.Is(err, errScheduleSelection):
		return "Enter team IDs or choose at least one discovered player."
	default:
		return err.Error()
	}
}

func selectedScheduleGames(games []Game, selectedIDs map[string]struct{}) []Game {
	filteredGames := make([]Game, 0, len(games))
	for i := range games {
		if _, ok := selectedIDs[games[i].ID]; ok {
			filteredGames = append(filteredGames, games[i])
		}
	}
	return filteredGames
}

func insertGoogleCalendarEvents(r *http.Request, record *googleConnectionRecord, token *oauth2.Token, games []Game) (int, int, bool, error) {
	added := 0
	existing := 0
	for i := range games {
		event, ok := googleEventPayload(r, &games[i])
		if !ok {
			continue
		}
		resp, err := googleInsertCalendarEvent(googleHTTPContext(r.Context()), record.CalendarID, token, &event)
		if err != nil {
			return 0, 0, false, err
		}
		if resp.StatusCode == http.StatusConflict {
			resp.Body.Close()
			existing++
			continue
		}
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			authRejected := isGoogleAuthRejection(resp)
			resp.Body.Close()
			return 0, 0, authRejected, nil
		}
		resp.Body.Close()
		added++
	}
	return added, existing, false, nil
}

func populateScheduleProps(ctx context.Context, session *SessionData, playerIDs []int, props *partials.SoccerTableFragmentProps) bool {
	games, fetchErr := resolveScheduleGames(ctx, session, playerIDs, nil)
	message, hint, clearSession := scheduleFetchFeedback(fetchErr)
	if fetchErr != nil && !clearSession {
		log.Printf("soccer LPS fetch failed: %v", fetchErr)
	}
	if fetchErr != nil {
		props.Message = message
		props.Hint = hint
	} else {
		props.Games = games
	}
	return clearSession
}

func resolveScheduleData(ctx context.Context, session *SessionData, playerIDs []int, teamCodes string, rawPlayerIDs []string, props *partials.SoccerTableFragmentProps) bool {
	if len(nonEmptyStrings(rawPlayerIDs)) > 0 && len(playerIDs) == 0 {
		props.Message = "One or more selected players were invalid."
		props.Hint = "Clear the imported players and import again to refresh the discovered player list."
		return false
	}
	if session != nil && len(playerIDs) > 0 {
		return populateScheduleProps(ctx, session, playerIDs, props)
	}
	if len(playerIDs) > 0 {
		props.Message = "Import a bearer JWT again to fetch schedules for your discovered players."
		props.Hint = "Your previous session is no longer available."
		return false
	}
	if strings.TrimSpace(teamCodes) != "" {
		teamIDs := parseTeamIDs(teamCodes)
		if len(teamIDs) == 0 {
			props.Message = "One or more team IDs were invalid."
			props.Hint = "Enter numeric Let's Play Soccer team IDs separated by commas."
			return false
		}
		games, fetchErr := resolveScheduleGames(ctx, session, playerIDs, teamIDs)
		message, hint, clearSession := scheduleFetchFeedback(fetchErr)
		if fetchErr != nil && !clearSession {
			log.Printf("soccer LPS fetch failed: %v", fetchErr)
		}
		if fetchErr != nil {
			props.Message = message
			props.Hint = hint
			return clearSession
		}
		props.Games = games
		return false
	}
	props.Message = "Enter team IDs or choose at least one discovered player."
	props.Hint = "Manual team ID entry still works if you do not want to import a token."
	return false
}

func resolveScheduleGames(ctx context.Context, session *SessionData, playerIDs, teamIDs []int) ([]Game, error) {
	switch {
	case session != nil && len(playerIDs) > 0:
		return lpsFetchGamesForPlayers(ctx, session.JWT, playerIDs)
	case len(teamIDs) > 0:
		return lpsFetchGamesForTeams(ctx, teamIDs)
	default:
		return nil, nil
	}
}

func handleScheduleDownloadError(w http.ResponseWriter, r *http.Request, err error) {
	_, _, shouldClearSession := scheduleFetchFeedback(err)
	if shouldClearSession {
		clearSession(w, r)
	}
	status, message := scheduleDownloadError(err)
	if status == http.StatusUnauthorized || status == http.StatusBadRequest {
		clearSession(w, r)
	}
	http.Error(w, message, status)
}

func fetchSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	teamCodes := r.FormValue("team_codes")
	rawPlayerIDs := r.Form["player_ids"]
	playerIDs := parsePlayerIDs(r.Form["player_ids"])
	session, swapAuthState := loadSoccerSession(w, r)

	googleConnected := false
	if googleEnabled() {
		record, googleErr := loadGoogleConnectionRecord(r.Context(), r)
		if googleErr != nil {
			log.Printf("google connection read failed: %v", googleErr)
			clearGoogleConnectionCookie(w, r)
		} else {
			googleConnected = record != nil
		}
	}
	props := partials.SoccerTableFragmentProps{TeamCodes: teamCodes, PlayerIDs: playerIDs, GoogleConnected: googleConnected}
	if resolveScheduleData(r.Context(), session, playerIDs, teamCodes, rawPlayerIDs, &props) {
		clearSession(w, r)
		swapAuthState = true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if swapAuthState {
		if err := partials.SoccerLoginState(soccerLoginStateProps(w, r, nil, true)).Render(context.Background(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := partials.SoccerTableFragment(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := io.WriteString(w, `<div class="subscribe-success">✅ Subscribed! Check your email to confirm.</div>`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func splitDelimitedValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
}

func parseTeamIDs(raw string) []int {
	return parsePlayerIDs(splitDelimitedValues(raw))
}

func downloadICSHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	selectedIDs := make(map[string]struct{})
	for _, selectedID := range r.Form["selected"] {
		selectedID = strings.TrimSpace(selectedID)
		if selectedID != "" {
			selectedIDs[selectedID] = struct{}{}
		}
	}
	if len(selectedIDs) == 0 {
		http.Error(w, "select at least one game", http.StatusBadRequest)
		return
	}

	teamCodes := r.FormValue("team_codes")
	rawPlayerIDs := r.Form["player_ids"]
	playerIDs := parsePlayerIDs(r.Form["player_ids"])
	if len(nonEmptyStrings(rawPlayerIDs)) > 0 && len(playerIDs) == 0 {
		http.Error(w, "one or more selected players were invalid; clear the imported players and import again to refresh the discovered player list", http.StatusBadRequest)
		return
	}
	session, _ := loadSoccerSession(w, r)

	games, err := requestedScheduleGames(r.Context(), session, playerIDs, teamCodes)
	if errors.Is(err, errPlayerSessionRequired) {
		http.Error(w, "import a bearer JWT again before downloading schedules for your discovered players", http.StatusUnauthorized)
		return
	}
	if errors.Is(err, errInvalidTeamSelection) {
		http.Error(w, "one or more team IDs were invalid; enter numeric Let's Play Soccer team IDs separated by commas", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Printf("soccer LPS fetch failed: %v", err)
		handleScheduleDownloadError(w, r, err)
		return
	}

	filteredGames := make([]Game, 0, len(games))
	for i := range games {
		if _, ok := selectedIDs[games[i].ID]; ok {
			filteredGames = append(filteredGames, games[i])
		}
	}
	if len(filteredGames) == 0 {
		http.Error(w, "no selected games were found", http.StatusBadRequest)
		return
	}

	icsContent := buildICS(filteredGames)

	w.Header().Set("Content-Type", "text/calendar")
	w.Header().Set("Content-Disposition", "attachment; filename=soccer_schedule.ics")
	_, err = io.WriteString(w, icsContent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func loginEnabled() bool {
	return len(configData.SessionKey) == 32
}

func syncGoogleCalendarSelection(ctx context.Context, record *googleConnectionRecord, calendars []GoogleCalendarOption) (calendarID, summary string) {
	calendarID = strings.TrimSpace(record.CalendarID)
	if calendarID == "" {
		calendarID, summary = preferredGoogleCalendar(calendars)
		record.CalendarID = calendarID
		record.CalendarSummary = summary
		record.UpdatedAt = time.Now().UTC()
		if err := currentGoogleConnectionStore().Put(ctx, record); err != nil {
			log.Printf("google connection default calendar save failed: %v", err)
		}
		return calendarID, summary
	}
	summary = googleCalendarSummary(calendars, calendarID)
	if summary == "" {
		calendarID, summary = preferredGoogleCalendar(calendars)
	}
	if summary != "" && (record.CalendarID != calendarID || record.CalendarSummary != summary) {
		record.CalendarID = calendarID
		record.CalendarSummary = summary
		record.UpdatedAt = time.Now().UTC()
		if err := currentGoogleConnectionStore().Put(ctx, record); err != nil {
			log.Printf("google connection calendar sync failed: %v", err)
		}
	}
	return calendarID, summary
}

func googleEnabled() bool {
	return loginEnabled() &&
		strings.TrimSpace(configData.GoogleClientID) != "" &&
		strings.TrimSpace(configData.GoogleClientSecret) != "" &&
		strings.TrimSpace(configData.GoogleConnectionTableName) != ""
}

func soccerLoginStateProps(w http.ResponseWriter, r *http.Request, session *SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	props := partials.SoccerLoginStateProps{
		Authenticated:   session != nil,
		GoogleAvailable: googleEnabled(),
		LoginAvailable:  loginEnabled(),
		SwapOOB:         swapOOB,
	}
	if session != nil {
		props.UserName = session.UserName
		props.Players = session.Players
	}
	if !props.GoogleAvailable {
		return props
	}
	record, err := loadGoogleConnectionRecord(r.Context(), r)
	if err != nil {
		log.Printf("google connection read failed: %v", err)
		clearGoogleConnectionCookie(w, r)
		return props
	}
	if record == nil {
		return props
	}
	calendars, err := googleListCalendars(r.Context(), r, record)
	if err != nil {
		log.Printf("google calendar list failed: %v", err)
		var apiErr *googleAPIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			deleteGoogleConnection(r.Context(), w, r)
		}
		return props
	}
	props.GoogleConnected = true
	props.GoogleCalendars = calendars
	props.SelectedGoogleCalendarID, props.GoogleCalendarSummary = syncGoogleCalendarSelection(r.Context(), record, calendars)
	return props
}

func renderSoccerLoginState(w http.ResponseWriter, r *http.Request, session *SessionData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	props := soccerLoginStateProps(w, r, session, false)
	if err := partials.SoccerLoginState(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func renderSoccerLoginFeedback(w http.ResponseWriter, kind, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	role := "status"
	if kind == "error" {
		role = "alert"
	}
	_, _ = io.WriteString(w, fmt.Sprintf(`<div class="soccer-login-message soccer-login-message-%s" role="%s">%s</div>`, kind, role, html.EscapeString(message)))
}

func encryptJSONValue(data any) (string, error) {
	if !loginEnabled() {
		return "", errors.New("session encryption key is not configured")
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(configData.SessionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, payload, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decryptJSONValue(value string, out any) error {
	if !loginEnabled() {
		return errors.New("session encryption key is not configured")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(configData.SessionKey)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(decoded) < gcm.NonceSize() {
		return errors.New("invalid session payload")
	}
	nonce := decoded[:gcm.NonceSize()]
	ciphertext := decoded[gcm.NonceSize():]
	payload, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return err
	}
	return nil
}

func encryptSession(data *SessionData) (string, error) {
	return encryptJSONValue(data)
}

func decryptSession(value string) (SessionData, error) {
	var session SessionData
	if err := decryptJSONValue(value, &session); err != nil {
		return session, err
	}
	return session, nil
}

func getSession(r *http.Request) (*SessionData, error) {
	cookie, err := r.Cookie(lpsSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	session, err := decryptSession(cookie.Value)
	if err != nil {
		return nil, err
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return nil, errSessionExpired
	}
	return &session, nil
}

func loadSoccerSession(w http.ResponseWriter, r *http.Request) (*SessionData, bool) {
	session, err := getSession(r)
	if errors.Is(err, errSessionExpired) {
		clearSession(w, r)
		return nil, true
	}
	if err != nil {
		log.Printf("soccer session read failed: %v", err)
		clearSession(w, r)
		return nil, true
	}
	return session, false
}

func setSession(w http.ResponseWriter, r *http.Request, session *SessionData) error {
	encrypted, err := encryptSession(session)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     lpsSessionCookieName,
		Value:    encrypted,
		Path:     "/soccer",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func clearSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     lpsSessionCookieName,
		Value:    "",
		Path:     "/soccer",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func getGoogleConnectionID(r *http.Request) string {
	cookie, err := r.Cookie(googleConnectionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func setGoogleConnectionCookie(w http.ResponseWriter, r *http.Request, connectionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleConnectionCookieName,
		Value:    connectionID,
		Path:     "/soccer",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(googleConnectionCookieTTL),
	})
}

func clearGoogleConnectionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleConnectionCookieName,
		Value:    "",
		Path:     "/soccer",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func setGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request, state googleOAuthState) error {
	encrypted, err := encryptJSONValue(state)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthStateCookieName,
		Value:    encrypted,
		Path:     "/soccer",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  state.ExpiresAt,
	})
	return nil
}

func getGoogleOAuthStateCookie(r *http.Request) (*googleOAuthState, error) {
	cookie, err := r.Cookie(googleOAuthStateCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state googleOAuthState
	if err := decryptJSONValue(cookie.Value, &state); err != nil {
		return nil, err
	}
	if time.Now().After(state.ExpiresAt) {
		return nil, errSessionExpired
	}
	return &state, nil
}

func clearGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     googleOAuthStateCookieName,
		Value:    "",
		Path:     "/soccer",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if remoteIP := remoteAddrIP(r.RemoteAddr); remoteIP != nil && isTrustedProxyIP(remoteIP) {
		return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
	}
	return false
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimSpace(r.Host)
}

func soccerGoogleFlashKind(code string) string {
	switch strings.TrimSpace(code) {
	case "failed", "denied", "unavailable":
		return "error"
	default:
		return "success"
	}
}

func soccerGoogleFlashMessage(code string) string {
	switch strings.TrimSpace(code) {
	case "connected":
		return "Google Calendar connected. Choose a calendar below and add selected games directly from the schedule table."
	case "denied":
		return "Google Calendar connection was canceled before access was granted."
	case "disconnected":
		return "Google Calendar connection removed."
	case "failed":
		return "Google Calendar connection could not be completed. Try again."
	case "unavailable":
		return "Google Calendar add is unavailable until Google OAuth and server-side storage are configured."
	default:
		return ""
	}
}

func newRandomHex(byteLength int) (string, error) {
	random := make([]byte, byteLength)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

func newGoogleOAuthState(connectionID string) (googleOAuthState, error) {
	stateValue, err := newRandomHex(16)
	if err != nil {
		return googleOAuthState{}, err
	}
	return googleOAuthState{
		ConnectionID: connectionID,
		ExpiresAt:    time.Now().Add(googleOAuthStateTTL),
		State:        stateValue,
	}, nil
}

func googleOAuthConfigForRequest(r *http.Request) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     configData.GoogleClientID,
		ClientSecret: configData.GoogleClientSecret,
		RedirectURL:  requestBaseURL(r) + "/soccer",
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar.events",
			"https://www.googleapis.com/auth/calendar.calendarlist.readonly",
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  googleOAuthAuthURL,
			TokenURL: googleOAuthTokenURL,
		},
	}
}

func googleHTTPContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, lpsHTTPClient)
}

func encryptGoogleToken(token *oauth2.Token) (string, error) {
	return encryptJSONValue(token)
}

func decryptGoogleToken(ciphertext string) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := decryptJSONValue(ciphertext, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func loadGoogleConnectionRecord(ctx context.Context, r *http.Request) (*googleConnectionRecord, error) {
	connectionID := getGoogleConnectionID(r)
	if connectionID == "" {
		return nil, nil
	}
	return currentGoogleConnectionStore().Get(ctx, connectionID)
}

func deleteGoogleConnection(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	connectionID := getGoogleConnectionID(r)
	if connectionID != "" {
		if err := currentGoogleConnectionStore().Delete(ctx, connectionID); err != nil {
			log.Printf("google connection delete failed: %v", err)
		}
	}
	clearGoogleConnectionCookie(w, r)
}

func currentGoogleToken(ctx context.Context, r *http.Request, record *googleConnectionRecord) (*oauth2.Token, error) {
	storedToken, err := decryptGoogleToken(record.TokenCiphertext)
	if err != nil {
		return nil, err
	}
	tokenSource := googleOAuthConfigForRequest(r).TokenSource(googleHTTPContext(ctx), storedToken)
	token, err := tokenSource.Token()
	if err != nil {
		return nil, err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = storedToken.RefreshToken
	}
	if token.AccessToken != storedToken.AccessToken || token.RefreshToken != storedToken.RefreshToken || !token.Expiry.Equal(storedToken.Expiry) {
		encryptedToken, encryptErr := encryptGoogleToken(token)
		if encryptErr != nil {
			return nil, encryptErr
		}
		record.TokenCiphertext = encryptedToken
		record.UpdatedAt = time.Now().UTC()
		if err := currentGoogleConnectionStore().Put(ctx, record); err != nil {
			return nil, err
		}
	}
	return token, nil
}

func newGoogleAPIRequest(ctx context.Context, method, requestPath string, query url.Values, token *oauth2.Token, body any) (*http.Request, error) {
	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		requestBody = bytes.NewReader(payload)
	}
	endpoint, err := url.JoinPath(googleCalendarAPIBaseURL, requestPath)
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody) //nolint:gosec // endpoint is derived from the constant Google Calendar API base URL and fixed paths.
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func googleInsertCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, event *googleEvent) (*http.Response, error) {
	req, err := newGoogleAPIRequest(ctx, http.MethodPost, "calendars/"+url.PathEscape(calendarID)+"/events", url.Values{"sendUpdates": {"none"}}, token, event)
	if err != nil {
		return nil, err
	}
	return lpsHTTPClient.Do(req) //nolint:gosec // request is created from the constant Google Calendar API base URL and fixed paths.
}

func readGoogleAPIError(resp *http.Response) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return &googleAPIError{
		StatusCode: resp.StatusCode,
		Message:    message,
	}
}

func googleListCalendarsWithToken(ctx context.Context, token *oauth2.Token) ([]GoogleCalendarOption, error) {
	req, err := newGoogleAPIRequest(ctx, http.MethodGet, "users/me/calendarList", url.Values{"minAccessRole": {"writer"}}, token, nil)
	if err != nil {
		return nil, err
	}
	resp, err := lpsHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, readGoogleAPIError(resp)
	}
	defer resp.Body.Close()
	var response googleCalendarListResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&response); err != nil {
		return nil, err
	}
	options := make([]GoogleCalendarOption, 0, len(response.Items))
	for _, item := range response.Items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Summary) == "" {
			continue
		}
		options = append(options, GoogleCalendarOption{
			ID:      item.ID,
			Primary: item.Primary,
			Summary: item.Summary,
		})
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Primary != options[j].Primary {
			return options[i].Primary
		}
		return strings.ToLower(options[i].Summary) < strings.ToLower(options[j].Summary)
	})
	return options, nil
}

func googleListCalendars(ctx context.Context, r *http.Request, record *googleConnectionRecord) ([]GoogleCalendarOption, error) {
	token, err := currentGoogleToken(ctx, r, record)
	if err != nil {
		return nil, err
	}
	return googleListCalendarsWithToken(googleHTTPContext(ctx), token)
}

func preferredGoogleCalendar(calendars []GoogleCalendarOption) (string, string) {
	for _, calendar := range calendars {
		if calendar.Primary {
			return calendar.ID, calendar.Summary
		}
	}
	if len(calendars) == 0 {
		return "", ""
	}
	return calendars[0].ID, calendars[0].Summary
}

func googleCalendarSummary(calendars []GoogleCalendarOption, calendarID string) string {
	for _, calendar := range calendars {
		if calendar.ID == calendarID {
			return calendar.Summary
		}
	}
	return ""
}

func googleEventID(game *Game) string {
	stableID := fallbackGameID(game)
	if stableID == "" {
		hash := md5.Sum([]byte(gameKey(game)))
		stableID = hex.EncodeToString(hash[:])
	}
	// Google Calendar event IDs only allow lowercase a-v and digits 0-9
	return "soccer" + stableID
}

func googleEventPayload(r *http.Request, game *Game) (googleEvent, bool) {
	start, end, ok := scheduleTimes(game)
	if !ok {
		return googleEvent{}, false
	}
	event := googleEvent{
		Description: strings.TrimSpace("Season " + game.Season),
		End: googleEventDateTime{
			DateTime: end.Format(time.RFC3339),
		},
		ID:       googleEventID(game),
		Location: strings.TrimSpace(game.Location),
		Start: googleEventDateTime{
			DateTime: start.Format(time.RFC3339),
		},
		Summary: strings.TrimSpace("Soccer: " + strings.TrimSpace(game.Home) + " vs " + strings.TrimSpace(game.Away)),
	}
	if event.Location == "" && strings.TrimSpace(game.Field) != "" {
		event.Location = "Field " + strings.TrimSpace(game.Field)
	}
	event.ExtendedProperties.Private = map[string]string{
		"portfolio_game_id": fallbackGameID(game),
		"portfolio_source":  "soccer",
	}
	event.Source = &googleEventSource{
		Title: "Craig Johnson Soccer Schedule",
		URL:   requestBaseURL(r) + "/soccer",
	}
	return event, true
}

func jwtExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

func normalizeImportedJWT(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("Paste the bearer JWT from your Let's Play Soccer browser session.")
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("Paste a single JWT value without extra spaces or line breaks.")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("The imported value must be a JWT with three dot-separated sections.")
	}
	for _, segment := range parts[:2] {
		if segment == "" {
			return "", errors.New("The imported value must be a JWT with three dot-separated sections.")
		}
		if _, err := base64.RawURLEncoding.DecodeString(segment); err != nil {
			return "", errors.New("The imported JWT format is not valid base64url data.")
		}
	}

	expiresAt := jwtExpiry(token)
	if !expiresAt.IsZero() && time.Now().After(expiresAt) {
		return "", errors.New("This JWT has expired. Copy a fresh bearer token from letsplaysoccer.com and import it again.")
	}

	return token, nil
}

func importedSessionExpiry(token string) time.Time {
	deadline := time.Now().Add(defaultSessionTTL)
	expiresAt := jwtExpiry(token)
	if expiresAt.IsZero() || expiresAt.After(deadline) {
		return deadline
	}
	return expiresAt
}

type lpsUserPlayerDiscovery struct {
	UserName string
	Players  []LPSPlayer
}

func lpsFetchUserPlayers(ctx context.Context, jwt string) (lpsUserPlayerDiscovery, error) {
	var discovery lpsUserPlayerDiscovery

	normalizedJWT, err := normalizeImportedJWT(jwt)
	if err != nil {
		return discovery, newLPSFetchError(lpsErrorMalformedToken, 0, http.StatusUnauthorized, "the imported JWT is malformed: %v", err)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, normalizedJWT, "users", "check")
	if err != nil {
		return discovery, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, http.StatusBadGateway, "could not reach Let's Play Soccer while loading players: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, http.StatusBadGateway, "could not read the player lookup response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return discovery, newLPSFetchError(lpsErrorUnauthorized, 0, resp.StatusCode, "Let's Play Soccer rejected the imported token with status 401")
	}
	if resp.StatusCode == http.StatusForbidden {
		return discovery, newLPSFetchError(lpsErrorForbidden, 0, resp.StatusCode, "Let's Play Soccer denied access to the player lookup with status 403")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, resp.StatusCode, "Let's Play Soccer returned status %d while loading players", resp.StatusCode)
	}

	discovery, err = decodeLPSUserPlayers(responseBody)
	if err != nil {
		var fetchErr *lpsFetchError
		if errors.As(err, &fetchErr) {
			return discovery, err
		}
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, http.StatusBadGateway, "%v", err)
	}

	return discovery, nil
}

func lpsFetchGamesForPlayers(ctx context.Context, jwt string, playerIDs []int) ([]Game, error) {
	normalizedJWT, err := normalizeImportedJWT(jwt)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorMalformedToken, 0, http.StatusUnauthorized, "the imported JWT is malformed: %v", err)
	}
	games := make([]Game, 0)
	indexByKey := make(map[string]int)
	for _, playerID := range playerIDs {
		playerGames, err := lpsFetchUpcomingGames(ctx, normalizedJWT, playerID)
		if err != nil {
			return nil, err
		}
		games = mergeScheduleGames(games, playerGames, indexByKey)
	}
	sortScheduleGames(games)
	return games, nil
}

func lpsFetchGamesForTeams(ctx context.Context, teamIDs []int) ([]Game, error) {
	games := make([]Game, 0)
	indexByKey := make(map[string]int)
	for _, teamID := range teamIDs {
		teamGames, err := lpsFetchTeamGames(ctx, teamID)
		if err != nil {
			return nil, err
		}
		games = mergeScheduleGames(games, teamGames, indexByKey)
	}
	sortScheduleGames(games)
	return games, nil
}

type lpsUserCheckResponse struct {
	AuthFailure bool        `json:"authFailure"`
	Error       string      `json:"error"`
	FirstName   string      `json:"first_name"`
	LastName    string      `json:"last_name"`
	Players     []LPSPlayer `json:"players"`
	UserPlayers []struct {
		PlayerID int  `json:"player_id"`
		Deleted  bool `json:"deleted"`
	} `json:"user_players"`
}

func lpsFetchUpcomingGames(ctx context.Context, normalizedJWT string, playerID int) ([]Game, error) {
	if playerID <= 0 {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, http.StatusBadRequest, "player ID %d is invalid", playerID)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, normalizedJWT, "players", strconv.Itoa(playerID), "upcoming_games")
	if err != nil {
		return nil, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading schedules: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not read the schedule response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, newLPSFetchError(lpsErrorUnauthorized, playerID, resp.StatusCode, "Let's Play Soccer rejected the imported token for player %d with status 401", playerID)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, newLPSFetchError(lpsErrorForbidden, playerID, resp.StatusCode, "Let's Play Soccer denied access to player %d with status 403", playerID)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, resp.StatusCode, "Let's Play Soccer could not find upcoming games for player %d", playerID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, resp.StatusCode, "Let's Play Soccer returned status %d while loading schedules", resp.StatusCode)
	}

	games, err := decodeLPSGames(responseBody)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "%v", err)
	}
	normalizeScheduleGames(games)
	return games, nil
}

func lpsFetchTeamGames(ctx context.Context, teamID int) ([]Game, error) {
	if teamID <= 0 {
		return nil, newLPSFetchError(lpsErrorInvalidTeam, teamID, http.StatusBadRequest, "team ID %d is invalid", teamID)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, "", "teams", strconv.Itoa(teamID))
	if err != nil {
		return nil, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, teamID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading team schedules: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, teamID, http.StatusBadGateway, "could not read the team schedule response: %w", err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return nil, newLPSFetchError(lpsErrorInvalidTeam, teamID, resp.StatusCode, "Let's Play Soccer could not find team %d", teamID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newLPSFetchError(lpsErrorUpstream, teamID, resp.StatusCode, "Let's Play Soccer returned status %d while loading team schedules", resp.StatusCode)
	}

	games, err := decodeLPSGames(responseBody)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, teamID, http.StatusBadGateway, "%v", err)
	}
	normalizeScheduleGames(games)
	return upcomingScheduleGames(games), nil
}

func decodeLPSUserPlayers(payload []byte) (lpsUserPlayerDiscovery, error) {
	var discovery lpsUserPlayerDiscovery

	var envelope lpsUserCheckResponse
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return discovery, errors.New("The player lookup response format was not recognized.")
	}
	if envelope.AuthFailure {
		message := strings.TrimSpace(envelope.Error)
		if message == "" {
			message = "Let's Play Soccer rejected the imported token."
		}
		return discovery, newLPSFetchError(lpsErrorUnauthorized, 0, http.StatusUnauthorized, "%s", message)
	}
	discovery.UserName = fullName(strings.TrimSpace(envelope.FirstName), strings.TrimSpace(envelope.LastName))
	if discovery.UserName == "" {
		discovery.UserName = "Let's Play Soccer account"
	}
	if len(envelope.Players) == 0 {
		discovery.Players = []LPSPlayer{}
		return discovery, nil
	}

	deletedPlayerIDs := make(map[int]struct{})
	for _, userPlayer := range envelope.UserPlayers {
		if userPlayer.Deleted {
			deletedPlayerIDs[userPlayer.PlayerID] = struct{}{}
		}
	}
	if len(deletedPlayerIDs) == 0 {
		discovery.Players = envelope.Players
		return discovery, nil
	}

	players := make([]LPSPlayer, 0, len(envelope.Players))
	for _, player := range envelope.Players {
		if _, deleted := deletedPlayerIDs[player.UPlayerID]; deleted {
			continue
		}
		players = append(players, player)
	}

	discovery.Players = players
	return discovery, nil
}

func fullName(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, " ")
}

func decodeLPSGames(payload []byte) ([]Game, error) {
	var envelope LambdaGamesResponse
	if err := json.Unmarshal(payload, &envelope); err == nil && len(envelope.Games) > 0 {
		return envelope.Games, nil
	}

	var raw any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, errors.New("The schedule response format was not recognized.")
	}

	items := extractGameMaps(raw)
	if len(items) == 0 {
		return []Game{}, nil
	}
	games := make([]Game, 0, len(items))
	for _, item := range items {
		game := mapLPSGame(item)
		if game.ID == "" && game.DateTime == "" && game.Home == "" && game.Away == "" {
			continue
		}
		games = append(games, game)
	}
	return games, nil
}

func extractGameMaps(raw any) []map[string]any {
	switch value := raw.(type) {
	case []any:
		games := make([]map[string]any, 0, len(value))
		for _, item := range value {
			if mapped, ok := item.(map[string]any); ok {
				games = append(games, mapped)
			}
		}
		return games
	case map[string]any:
		for _, key := range []string{"games", "upcoming_games", "data", "results", "items"} {
			if nested, ok := value[key]; ok {
				games := extractGameMaps(nested)
				if len(games) > 0 {
					return games
				}
			}
		}
		return []map[string]any{value}
	default:
		return nil
	}
}

func mapLPSGame(raw map[string]any) Game {
	startAt := firstString(raw,
		"start_at", "starts_at", "start_datetime", "StartDateTime", "SchedGameDateTime", "schedGameDateTime", "game_datetime", "datetime", "date_time",
	)
	endAt := firstString(raw,
		"end_at", "ends_at", "end_datetime", "EndDateTime", "schedGameEndTime", "SchedGameEndTime", "game_end_datetime", "end_time",
	)
	dateTime := firstString(raw, "display_datetime", "DisplayDateTime", "DateTime", "datetime", "date_time")
	if dateTime == "" {
		dateTime = formatGameDateTime(startAt)
	}

	homeTeam := firstString(raw, "home", "Home", "home_team", "HomeTeam", "home_team_name", "TeamName")
	awayTeam := firstString(raw, "away", "Away", "away_team", "visitor_team", "AwayTeam", "away_team_name", "visitor_team_name", "OpponentName", "opponent_name")
	if homeTeam == "" && awayTeam == "" {
		matchup := firstString(raw, "matchup", "Matchup", "title", "Title")
		if matchup != "" {
			parts := strings.Split(matchup, " vs ")
			if len(parts) == 2 {
				homeTeam = strings.TrimSpace(parts[0])
				awayTeam = strings.TrimSpace(parts[1])
			}
		}
	}

	return Game{
		ID:       firstString(raw, "id", "ID", "game_id", "GameID", "UGameID"),
		DateTime: dateTime,
		StartAt:  startAt,
		EndAt:    endAt,
		Field:    firstString(raw, "field_name", "FieldName", "field", "Field"),
		Location: firstString(raw, "location", "Location", "venue", "Venue", "facility", "Facility", "facilityName"),
		Home:     homeTeam,
		Away:     awayTeam,
		Season:   firstString(raw, "season", "Season", "season_id", "SeasonID"),
	}
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if converted := anyToString(value); converted != "" {
			return converted
		}
	}
	return ""
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case json.Number:
		return typed.String()
	case map[string]any:
		for _, nestedKey := range []string{"name", "Name", "title", "Title", "value", "Value", "team_name", "TeamName", "display_name", "DisplayName"} {
			if nestedValue, ok := typed[nestedKey]; ok {
				if converted := anyToString(nestedValue); converted != "" {
					return converted
				}
			}
		}
	}
	return ""
}

func parseSelectedIDs(form url.Values) map[string]struct{} {
	selectedIDs := make(map[string]struct{})
	for _, id := range form["selected"] {
		id = strings.TrimSpace(id)
		if id != "" {
			selectedIDs[id] = struct{}{}
		}
	}
	return selectedIDs
}

func parsePlayerIDs(values []string) []int {
	seen := make(map[int]struct{})
	playerIDs := make([]int, 0, len(values))
	for _, value := range values {
		playerID, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || playerID <= 0 {
			continue
		}
		if _, exists := seen[playerID]; exists {
			continue
		}
		seen[playerID] = struct{}{}
		playerIDs = append(playerIDs, playerID)
	}
	return playerIDs
}

func nonEmptyStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func mergeGames(base, incoming *Game) Game {
	merged := *base
	if merged.ID == "" {
		merged.ID = incoming.ID
	}
	if merged.DateTime == "" {
		merged.DateTime = incoming.DateTime
	}
	if merged.StartAt == "" {
		merged.StartAt = incoming.StartAt
	}
	if merged.EndAt == "" {
		merged.EndAt = incoming.EndAt
	}
	if merged.Field == "" {
		merged.Field = incoming.Field
	}
	if merged.Location == "" {
		merged.Location = incoming.Location
	}
	if merged.Home == "" {
		merged.Home = incoming.Home
	}
	if merged.Away == "" {
		merged.Away = incoming.Away
	}
	if merged.Season == "" {
		merged.Season = incoming.Season
	}
	if merged.Location == "" && merged.Field != "" {
		merged.Location = "Field " + merged.Field
	}
	if merged.Field == "" && merged.Location != "" {
		merged.Field = merged.Location
	}
	if merged.DateTime == "" {
		merged.DateTime = formatGameDateTime(merged.StartAt)
	}
	if merged.ID == "" {
		merged.ID = fallbackGameID(&merged)
	}
	return merged
}

func stableGameFields(game *Game) string {
	return strings.Join([]string{game.Home, game.Away, game.StartAt, game.DateTime, game.Location, game.Season}, "|")
}

func fallbackGameID(game *Game) string {
	base := stableGameFields(game)
	if strings.ReplaceAll(base, "|", "") == "" {
		return ""
	}
	hash := md5.Sum([]byte(base))
	return hex.EncodeToString(hash[:])
}

func gameKey(game *Game) string {
	if game.ID != "" {
		return game.ID
	}
	return stableGameFields(game)
}

func gameStartTime(game *Game) (time.Time, bool) {
	if parsed, ok := parseFlexibleTime(game.StartAt); ok {
		return parsed, true
	}
	return parseFlexibleTime(game.DateTime)
}

func parseFlexibleTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []struct {
		layout   string
		location *time.Location
	}{
		{layout: time.RFC3339Nano},
		{layout: time.RFC3339},
		{layout: "2006-01-02T15:04:05.000Z", location: time.UTC},
		{layout: "2006-01-02T15:04:05Z", location: time.UTC},
		{layout: "2006-01-02T15:04:05", location: time.Local},
		{layout: "2006-01-02 15:04:05", location: time.Local},
		{layout: "2006-01-02 15:04", location: time.Local},
		{layout: "Mon 01/02/06 03:04 PM", location: time.Local},
	}
	for _, candidate := range layouts {
		var (
			parsed time.Time
			err    error
		)
		if candidate.location != nil {
			parsed, err = time.ParseInLocation(candidate.layout, value, candidate.location)
		} else {
			parsed, err = time.Parse(candidate.layout, value)
		}
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func formatGameDateTime(value string) string {
	parsed, ok := parseFlexibleTime(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return parsed.Format("Mon 01/02/06 03:04 PM")
}

func normalizeScheduleGames(games []Game) {
	for index := range games {
		if games[index].ID == "" {
			games[index].ID = fallbackGameID(&games[index])
		}
		if games[index].DateTime == "" {
			games[index].DateTime = formatGameDateTime(games[index].StartAt)
		}
		if games[index].Location == "" && games[index].Field != "" {
			games[index].Location = "Field " + games[index].Field
		}
		if games[index].Field == "" && games[index].Location != "" {
			games[index].Field = games[index].Location
		}
	}
}

func mergeScheduleGames(games, incoming []Game, indexByKey map[string]int) []Game {
	for i := range incoming {
		game := &incoming[i]
		key := gameKey(game)
		if existingIndex, exists := indexByKey[key]; exists {
			games[existingIndex] = mergeGames(&games[existingIndex], game)
			continue
		}
		indexByKey[key] = len(games)
		games = append(games, *game)
	}
	return games
}

func sortScheduleGames(games []Game) {
	sort.Slice(games, func(i, j int) bool {
		left, leftOK := gameStartTime(&games[i])
		right, rightOK := gameStartTime(&games[j])
		if leftOK && rightOK {
			if !left.Equal(right) {
				return left.Before(right)
			}
			return compareScheduleGames(&games[i], &games[j]) < 0
		}
		if games[i].DateTime != games[j].DateTime {
			return games[i].DateTime < games[j].DateTime
		}
		return compareScheduleGames(&games[i], &games[j]) < 0
	})
}

func compareScheduleGames(left, right *Game) int {
	for _, pair := range [][2]string{
		{left.DateTime, right.DateTime},
		{left.StartAt, right.StartAt},
		{left.Home, right.Home},
		{left.Away, right.Away},
		{left.Location, right.Location},
		{left.Field, right.Field},
		{left.Season, right.Season},
		{left.ID, right.ID},
	} {
		if pair[0] == pair[1] {
			continue
		}
		if pair[0] < pair[1] {
			return -1
		}
		return 1
	}
	return 0
}

func upcomingScheduleGames(games []Game) []Game {
	filtered := make([]Game, 0, len(games))
	now := time.Now()
	for i := range games {
		start, ok := gameStartTime(&games[i])
		if ok && start.Before(now) {
			continue
		}
		filtered = append(filtered, games[i])
	}
	return filtered
}

func buildICS(games []Game) string {
	var builder strings.Builder
	writeICSLine(&builder, "BEGIN:VCALENDAR")
	writeICSLine(&builder, "VERSION:2.0")
	writeICSLine(&builder, "PRODID:-//Craig Johnson Portfolio//Soccer Schedule//EN")
	for i := range games {
		game := &games[i]
		start, end, ok := scheduleTimes(game)
		if !ok {
			log.Printf("skipping game: could not parse start time")
			continue
		}
		writeICSLine(&builder, "BEGIN:VEVENT")
		writeICSLine(&builder, "UID:"+escapeICSText(game.ID)+"@craigdevjohnson.com")
		writeICSLine(&builder, "DTSTAMP:"+time.Now().UTC().Format("20060102T150405Z"))
		writeICSLine(&builder, "DTSTART:"+start.UTC().Format("20060102T150405Z"))
		writeICSLine(&builder, "DTEND:"+end.UTC().Format("20060102T150405Z"))
		writeICSLine(&builder, "SUMMARY:"+escapeICSText("Soccer: "+strings.TrimSpace(game.Home)+" vs "+strings.TrimSpace(game.Away)))
		location := strings.TrimSpace(game.Location)
		if location == "" && strings.TrimSpace(game.Field) != "" {
			location = "Field " + strings.TrimSpace(game.Field)
		}
		if location != "" {
			writeICSLine(&builder, "LOCATION:"+escapeICSText(location))
		}
		description := strings.TrimSpace("Season " + game.Season)
		if description != "Season" {
			writeICSLine(&builder, "DESCRIPTION:"+escapeICSText(description))
		}
		writeICSLine(&builder, "END:VEVENT")
	}
	writeICSLine(&builder, "END:VCALENDAR")
	return builder.String()
}

func scheduleTimes(game *Game) (time.Time, time.Time, bool) {
	start, ok := gameStartTime(game)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	end, ok := parseFlexibleTime(game.EndAt)
	if !ok || !end.After(start) {
		end = start.Add(90 * time.Minute)
	}
	return start, end, true
}

func escapeICSText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ";", `\;`)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func writeICSLine(builder *strings.Builder, line string) {
	const maxLineBytes = 75

	firstSegment := true
	for line != "" {
		available := maxLineBytes
		if !firstSegment {
			builder.WriteByte(' ')
			available--
		}

		written := 0
		for index := 0; index < len(line); {
			_, size := utf8.DecodeRuneInString(line[index:])
			if written > 0 && written+size > available {
				break
			}
			written += size
			index += size
		}

		builder.WriteString(line[:written])
		builder.WriteString("\r\n")
		line = line[written:]
		firstSegment = false
	}
}

func scheduleFetchFeedback(err error) (string, string, bool) {
	if err == nil {
		return "", "", false
	}
	var fetchErr *lpsFetchError
	if errors.As(err, &fetchErr) {
		if detail, ok := scheduleErrorDetail(fetchErr); ok {
			return detail.feedbackMessage, detail.feedbackHint, detail.clearSession
		}
	}
	if errors.Is(err, errSessionExpired) {
		return "Your imported Let's Play Soccer token expired.", "Copy a fresh bearer JWT from letsplaysoccer.com and import it again.", true
	}
	return "Could not load schedules from Let's Play Soccer right now.", "Try again in a moment, or use team IDs manually.", false
}

func scheduleDownloadError(err error) (int, string) {
	var fetchErr *lpsFetchError
	if errors.As(err, &fetchErr) {
		if detail, ok := scheduleErrorDetail(fetchErr); ok {
			return detail.downloadStatus, detail.downloadMessage
		}
	}
	return http.StatusBadGateway, "could not refresh the authenticated schedule"
}

type scheduleErrorDetails struct {
	clearSession    bool
	downloadMessage string
	downloadStatus  int
	feedbackHint    string
	feedbackMessage string
}

func scheduleErrorDetail(fetchErr *lpsFetchError) (scheduleErrorDetails, bool) {
	if fetchErr == nil {
		return scheduleErrorDetails{}, false
	}

	switch fetchErr.Kind {
	case lpsErrorMalformedToken:
		return scheduleErrorDetails{
			clearSession:    true,
			downloadMessage: "the imported Let's Play Soccer token is malformed; import the full bearer JWT again",
			downloadStatus:  http.StatusUnauthorized,
			feedbackHint:    "Copy the full bearer JWT from letsplaysoccer.com and import it again.",
			feedbackMessage: "The imported Let's Play Soccer token is not a valid JWT.",
		}, true
	case lpsErrorUnauthorized:
		return scheduleErrorDetails{
			clearSession:    true,
			downloadMessage: "your imported Let's Play Soccer token was rejected; import a fresh bearer JWT from letsplaysoccer.com",
			downloadStatus:  http.StatusUnauthorized,
			feedbackHint:    "Copy a fresh bearer JWT from letsplaysoccer.com and import it again.",
			feedbackMessage: "Your imported Let's Play Soccer token was rejected.",
		}, true
	case lpsErrorForbidden:
		return scheduleErrorDetails{
			clearSession:    false,
			downloadMessage: fmt.Sprintf("Let's Play Soccer denied access to discovered player %d; clear the imported players and import again", fetchErr.PlayerID),
			downloadStatus:  http.StatusForbidden,
			feedbackHint:    "Clear the imported players and import again to refresh the discovered player list.",
			feedbackMessage: fmt.Sprintf("Let's Play Soccer denied access to discovered player %d.", fetchErr.PlayerID),
		}, true
	case lpsErrorInvalidPlayer:
		return scheduleErrorDetails{
			clearSession:    false,
			downloadMessage: fmt.Sprintf("discovered player %d was not accepted by Let's Play Soccer; clear the imported players and import again", fetchErr.PlayerID),
			downloadStatus:  http.StatusBadRequest,
			feedbackHint:    "Clear the imported players and import again to refresh the discovered player list.",
			feedbackMessage: fmt.Sprintf("Discovered player %d was not accepted by Let's Play Soccer.", fetchErr.PlayerID),
		}, true
	case lpsErrorInvalidTeam:
		return scheduleErrorDetails{
			clearSession:    false,
			downloadMessage: fmt.Sprintf("team ID %d was not accepted by Let's Play Soccer; enter a valid numeric team ID and try again", fetchErr.PlayerID),
			downloadStatus:  http.StatusBadRequest,
			feedbackHint:    "Enter valid numeric team IDs from the Let's Play Soccer Team Schedules page and try again.",
			feedbackMessage: fmt.Sprintf("Team ID %d was not accepted by Let's Play Soccer.", fetchErr.PlayerID),
		}, true
	case lpsErrorUpstream:
		return scheduleErrorDetails{
			clearSession:    false,
			downloadMessage: "could not refresh the authenticated schedule because Let's Play Soccer is unavailable",
			downloadStatus:  http.StatusBadGateway,
			feedbackHint:    "Their API may be unavailable. Try again in a moment, or use team IDs manually.",
			feedbackMessage: "Could not load schedules from Let's Play Soccer right now.",
		}, true
	default:
		return scheduleErrorDetails{}, false
	}
}

func clientIP(r *http.Request) string {
	if ip, ok := forwardedClientIP(r); ok {
		return ip
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func forwardedClientIP(r *http.Request) (string, bool) {
	remoteIP := remoteAddrIP(r.RemoteAddr)
	if remoteIP == nil || !isTrustedProxyIP(remoteIP) {
		return "", false
	}

	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); isValidIP(ip) {
		return ip, true
	}

	for _, candidate := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		candidate = strings.TrimSpace(candidate)
		if isValidIP(candidate) {
			return candidate, true
		}
	}

	return "", false
}

func remoteAddrIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	return net.ParseIP(host)
}

func isTrustedProxyIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func isValidIP(value string) bool {
	return net.ParseIP(strings.TrimSpace(value)) != nil
}
