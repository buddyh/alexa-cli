package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Client is the Alexa API client
type Client struct {
	httpClient     *http.Client
	cookies        string
	csrf           string
	activityCSRF   string // separate CSRF for activity/history endpoints
	amazonDomain   string // e.g., "amazon.com"
	customerID     string
	bearerToken    string // Atna| token for AVS APIs
	conversationID string // Current conversation ID for Alexa+
	refreshToken   string // Store for re-auth
	verbose        bool   // Enable debug logging
}

// SetVerbose enables or disables verbose debug output
func (c *Client) SetVerbose(v bool) {
	c.verbose = v
}

func (c *Client) log(format string, args ...interface{}) {
	if c.verbose {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

// NewClient creates a new Alexa API client
func NewClient(refreshToken, amazonDomain string) (*Client, error) {
	client := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		amazonDomain: amazonDomain,
		refreshToken: refreshToken,
	}

	// Exchange refresh token for cookies
	if err := client.authenticate(refreshToken); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return client, nil
}

// authenticate exchanges a refresh token for session cookies
func (c *Client) authenticate(refreshToken string) error {
	// Amazon token exchange endpoint
	authURL := "https://api.amazon.com/ap/exchangetoken/cookies"

	data := url.Values{}
	data.Set("app_name", "Amazon Alexa")
	data.Set("requested_token_type", "auth_cookies")
	data.Set("source_token_type", "refresh_token")
	data.Set("source_token", refreshToken)
	data.Set("domain", "."+c.amazonDomain)

	req, err := http.NewRequest("POST", authURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-amzn-identity-auth-domain", "api."+c.amazonDomain)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Response struct {
			Tokens struct {
				Cookies map[string][]struct {
					Name  string `json:"Name"`
					Value string `json:"Value"`
				} `json:"cookies"`
			} `json:"tokens"`
		} `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	// Build cookie string from response
	var cookieParts []string
	for domain, cookies := range result.Response.Tokens.Cookies {
		_ = domain // cookies are keyed by domain
		for _, cookie := range cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", cookie.Name, cookie.Value))
		}
	}
	c.cookies = strings.Join(cookieParts, "; ")

	if c.cookies == "" {
		return fmt.Errorf("no cookies received from token exchange")
	}

	// Get CSRF token
	if err := c.fetchCSRF(); err != nil {
		return fmt.Errorf("failed to get CSRF token: %w", err)
	}

	return nil
}

// fetchCSRF retrieves the CSRF token from Amazon
func (c *Client) fetchCSRF() error {
	// Use the language API endpoint which returns CSRF as a cookie
	csrfURL := fmt.Sprintf("https://alexa.%s/api/language", c.amazonDomain)

	req, err := http.NewRequest("GET", csrfURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Cookie", c.cookies)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Extract csrf from response cookies
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "csrf" {
			c.csrf = cookie.Value
			// Also add csrf to our cookie string for future requests
			c.cookies = c.cookies + "; csrf=" + cookie.Value
			return nil
		}
	}

	// Try to extract from existing cookies (may already be present)
	for _, part := range strings.Split(c.cookies, "; ") {
		if strings.HasPrefix(part, "csrf=") {
			c.csrf = strings.TrimPrefix(part, "csrf=")
			return nil
		}
	}

	return fmt.Errorf("CSRF token not found")
}

// baseURL returns the Alexa API base URL
func (c *Client) baseURL() string {
	// pitangui for US, layla for EU/UK
	// The subdomain changes based on region, but the TLD matches the user's amazon domain
	switch c.amazonDomain {
	case "amazon.com":
		return "https://pitangui.amazon.com"
	case "amazon.co.uk":
		return "https://layla.amazon.co.uk"
	case "amazon.de":
		return "https://layla.amazon.de"
	case "amazon.fr":
		return "https://layla.amazon.fr"
	case "amazon.it":
		return "https://layla.amazon.it"
	case "amazon.es":
		return "https://layla.amazon.es"
	case "amazon.co.jp":
		return "https://layla.amazon.co.jp"
	case "amazon.com.au":
		return "https://alexa.amazon.com.au"
	case "amazon.ca":
		return "https://pitangui.amazon.ca"
	case "amazon.com.br":
		return "https://pitangui.amazon.com.br"
	case "amazon.in":
		return "https://pitangui.amazon.in"
	default:
		// Fall back to layla with the user's domain
		return fmt.Sprintf("https://layla.%s", c.amazonDomain)
	}
}

// alexaURL returns the alexa.amazon.com base URL
func (c *Client) alexaURL() string {
	return fmt.Sprintf("https://alexa.%s", c.amazonDomain)
}

// request makes an authenticated request to the Alexa API (pitangui/layla)
func (c *Client) request(method, endpoint string, body interface{}) ([]byte, error) {
	return c.doRequest(c.baseURL(), method, endpoint, body)
}

// requestAlexa makes an authenticated request to alexa.amazon.com
func (c *Client) requestAlexa(method, endpoint string, body interface{}) ([]byte, error) {
	return c.doRequest(c.alexaURL(), method, endpoint, body)
}

// doRequest makes an authenticated request to the specified base URL
func (c *Client) doRequest(baseURL, method, endpoint string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	var jsonBodyStr string
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		jsonBodyStr = string(jsonBody)
		reqBody = bytes.NewReader(jsonBody)
	}

	fullURL := baseURL + endpoint
	c.log("Request: %s %s", method, fullURL)
	if jsonBodyStr != "" {
		c.log("Request body: %s", jsonBodyStr[:min(500, len(jsonBodyStr))])
	}

	req, err := http.NewRequest(method, fullURL, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Cookie", c.cookies)
	req.Header.Set("csrf", c.csrf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log("Request error: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	c.log("Response status: %d, body length: %d", resp.StatusCode, len(respBody))
	if resp.StatusCode >= 400 {
		c.log("Error response: %s", string(respBody))
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Device represents an Alexa device
type Device struct {
	AccountName               string `json:"accountName"`
	SerialNumber              string `json:"serialNumber"`
	DeviceType                string `json:"deviceType"`
	DeviceFamily              string `json:"deviceFamily"`
	DeviceOwnerCustomerID     string `json:"deviceOwnerCustomerId"`
	Online                    bool   `json:"online"`
	Capabilities              []string `json:"capabilities"`
}

// GetDevices returns all Alexa devices
func (c *Client) GetDevices() ([]Device, error) {
	data, err := c.request("GET", "/api/devices-v2/device?cached=true", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Devices []Device `json:"devices"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse devices: %w", err)
	}

	// Store customer ID from first device
	if len(result.Devices) > 0 {
		c.customerID = result.Devices[0].DeviceOwnerCustomerID
	}

	return result.Devices, nil
}

// SequenceCommand sends a sequence command to a device
func (c *Client) SequenceCommand(device *Device, command string) error {
	// Ensure we have customer ID
	if c.customerID == "" {
		c.customerID = device.DeviceOwnerCustomerID
	}

	// Parse command type and build the appropriate payload
	var sequenceJson string

	switch {
	case strings.HasPrefix(command, "speak:"):
		text := strings.TrimPrefix(command, "speak:")
		text = strings.Trim(text, "'\"")

		sequenceJson = fmt.Sprintf(`{
			"@type": "com.amazon.alexa.behaviors.model.Sequence",
			"startNode": {
				"@type": "com.amazon.alexa.behaviors.model.OpaquePayloadOperationNode",
				"type": "Alexa.Speak",
				"operationPayload": {
					"deviceType": "%s",
					"deviceSerialNumber": "%s",
					"customerId": "%s",
					"locale": "%s",
					"textToSpeak": %s
				}
			}
		}`, device.DeviceType, device.SerialNumber, c.customerID, c.locale(), mustJSON(text))

	case strings.HasPrefix(command, "announcement:"):
		text := strings.TrimPrefix(command, "announcement:")
		text = strings.Trim(text, "'\"")

		sequenceJson = fmt.Sprintf(`{
			"@type": "com.amazon.alexa.behaviors.model.Sequence",
			"startNode": {
				"@type": "com.amazon.alexa.behaviors.model.OpaquePayloadOperationNode",
				"type": "AlexaAnnouncement",
				"operationPayload": {
					"expireAfter": "PT5S",
					"content": [{
						"locale": "%s",
						"display": {"title": "Announcement", "body": %s},
						"speak": {"type": "text", "value": %s}
					}],
					"target": {
						"customerId": "%s"
					}
				}
			}
		}`, c.locale(), mustJSON(text), mustJSON(text), c.customerID)

	case strings.HasPrefix(command, "textcommand:"):
		text := strings.TrimPrefix(command, "textcommand:")
		text = strings.Trim(text, "'\"")

		sequenceJson = fmt.Sprintf(`{
			"@type": "com.amazon.alexa.behaviors.model.Sequence",
			"startNode": {
				"@type": "com.amazon.alexa.behaviors.model.OpaquePayloadOperationNode",
				"type": "Alexa.TextCommand",
				"skillId": "amzn1.ask.1p.tellalexa",
				"operationPayload": {
					"deviceType": "%s",
					"deviceSerialNumber": "%s",
					"customerId": "%s",
					"locale": "%s",
					"text": %s
				}
			}
		}`, device.DeviceType, device.SerialNumber, c.customerID, c.locale(), mustJSON(text))

	case strings.HasPrefix(command, "automation:"):
		routineName := strings.TrimPrefix(command, "automation:")
		routineName = strings.Trim(routineName, "'\"")
		return c.ExecuteRoutine(device, routineName)

	default:
		return fmt.Errorf("unknown command type: %s", command)
	}

	payload := map[string]interface{}{
		"behaviorId":   "PREVIEW",
		"sequenceJson": sequenceJson,
		"status":       "ENABLED",
	}

	_, err := c.request("POST", "/api/behaviors/preview", payload)
	return err
}

// ExecuteRoutine runs an Alexa routine by name
func (c *Client) ExecuteRoutine(device *Device, routineName string) error {
	// First, get the list of routines
	routines, err := c.GetRoutines()
	if err != nil {
		return fmt.Errorf("failed to get routines: %w", err)
	}

	// Find matching routine
	var targetRoutine *Routine
	nameLower := strings.ToLower(routineName)
	for i, r := range routines {
		if strings.ToLower(r.Name) == nameLower {
			targetRoutine = &routines[i]
			break
		}
	}

	if targetRoutine == nil {
		return fmt.Errorf("routine '%s' not found", routineName)
	}

	// Execute the routine
	payload := map[string]interface{}{
		"behaviorId": targetRoutine.AutomationID,
		"sequenceJson": targetRoutine.Sequence,
		"status": "ENABLED",
	}

	_, err = c.request("POST", "/api/behaviors/preview", payload)
	return err
}

// Routine represents an Alexa routine
type Routine struct {
	AutomationID string `json:"automationId"`
	Name         string `json:"name"`
	Sequence     string `json:"sequence"`
}

// GetRoutines returns all Alexa routines
func (c *Client) GetRoutines() ([]Routine, error) {
	// Routines are on alexa.amazon.com, not pitangui
	data, err := c.requestAlexa("GET", "/api/behaviors/automations", nil)
	if err != nil {
		return nil, err
	}

	var rawRoutines []struct {
		AutomationID string `json:"automationId"`
		Name         string `json:"name"`
		Sequence     json.RawMessage `json:"sequence"`
	}

	if err := json.Unmarshal(data, &rawRoutines); err != nil {
		return nil, fmt.Errorf("failed to parse routines: %w", err)
	}

	routines := make([]Routine, len(rawRoutines))
	for i, r := range rawRoutines {
		routines[i] = Routine{
			AutomationID: r.AutomationID,
			Name:         r.Name,
			Sequence:     string(r.Sequence),
		}
	}

	return routines, nil
}

// SmartHomeDevice represents a smart home device
type SmartHomeDevice struct {
	EntityID     string `json:"entityId"`
	ApplianceID  string `json:"applianceId"`
	Name         string `json:"friendlyName"`
	Description  string `json:"friendlyDescription"`
	Type         string `json:"applianceTypes"`
	Reachable    bool   `json:"isReachable"`
}

// GetSmartHomeDevices returns all smart home devices
func (c *Client) GetSmartHomeDevices() ([]SmartHomeDevice, error) {
	data, err := c.request("GET", "/api/phoenix", nil)
	if err != nil {
		return nil, err
	}

	// Phoenix response has nested structure
	var result struct {
		NetworkDetail []struct {
			ApplianceDetails map[string]SmartHomeDevice `json:"applianceDetails"`
		} `json:"networkDetail"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse smart home devices: %w", err)
	}

	var devices []SmartHomeDevice
	for _, network := range result.NetworkDetail {
		for _, device := range network.ApplianceDetails {
			devices = append(devices, device)
		}
	}

	return devices, nil
}

// ControlSmartHome controls a smart home device
func (c *Client) ControlSmartHome(entityID string, action string, value interface{}) error {
	var payload map[string]interface{}

	switch action {
	case "on", "turnOn":
		payload = map[string]interface{}{
			"controlRequests": []map[string]interface{}{
				{
					"entityId":  entityID,
					"entityType": "APPLIANCE",
					"parameters": map[string]interface{}{
						"action": "turnOn",
					},
				},
			},
		}
	case "off", "turnOff":
		payload = map[string]interface{}{
			"controlRequests": []map[string]interface{}{
				{
					"entityId":  entityID,
					"entityType": "APPLIANCE",
					"parameters": map[string]interface{}{
						"action": "turnOff",
					},
				},
			},
		}
	case "brightness":
		payload = map[string]interface{}{
			"controlRequests": []map[string]interface{}{
				{
					"entityId":  entityID,
					"entityType": "APPLIANCE",
					"parameters": map[string]interface{}{
						"action": "setBrightness",
						"brightness": value,
					},
				},
			},
		}
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	_, err := c.request("PUT", "/api/phoenix/state", payload)
	return err
}

// fetchActivityCSRF retrieves the CSRF token needed for activity/history endpoints
func (c *Client) fetchActivityCSRF() error {
	activityURL := fmt.Sprintf("https://www.%s/alexa-privacy/apd/activity?ref=activityHistory", c.amazonDomain)

	req, err := http.NewRequest("GET", activityURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Cookie", c.cookies)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	bodyStr := string(body)

	// Try multiple patterns for the CSRF token
	// Pattern 1: meta tag
	re := regexp.MustCompile(`<meta name="csrf-token" content="([^"]+)"`)
	matches := re.FindStringSubmatch(bodyStr)
	if len(matches) >= 2 {
		c.activityCSRF = matches[1]
		return nil
	}

	// Pattern 2: data attribute
	re2 := regexp.MustCompile(`data-csrf="([^"]+)"`)
	matches = re2.FindStringSubmatch(bodyStr)
	if len(matches) >= 2 {
		c.activityCSRF = matches[1]
		return nil
	}

	// Pattern 3: JavaScript variable
	re3 := regexp.MustCompile(`"csrfToken"\s*:\s*"([^"]+)"`)
	matches = re3.FindStringSubmatch(bodyStr)
	if len(matches) >= 2 {
		c.activityCSRF = matches[1]
		return nil
	}

	// Pattern 4: anti-csrf in any form
	re4 := regexp.MustCompile(`anti-csrftoken-a2z['":\s]+['"]([^'"]+)['"]`)
	matches = re4.FindStringSubmatch(bodyStr)
	if len(matches) >= 2 {
		c.activityCSRF = matches[1]
		return nil
	}

	return fmt.Errorf("activity CSRF token not found in page")
}

// HistoryRecord represents a voice history record
type HistoryRecord struct {
	RecordKey       string    `json:"recordKey"`
	Timestamp       int64     `json:"timestamp"`
	Device          string    `json:"device"`
	CustomerUtterance string  `json:"customerUtterance"` // What you said (ASR)
	AlexaResponse   string    `json:"alexaResponse"`     // What Alexa said (TTS)
}

// GetCustomerHistoryRecords retrieves recent voice activity history
func (c *Client) GetCustomerHistoryRecords(startTime, endTime int64) ([]HistoryRecord, error) {
	// Ensure we have the activity CSRF token
	if c.activityCSRF == "" {
		if err := c.fetchActivityCSRF(); err != nil {
			return nil, fmt.Errorf("failed to get activity CSRF: %w", err)
		}
	}

	// Build URL with time range
	historyURL := fmt.Sprintf(
		"https://www.%s/alexa-privacy/apd/rvh/customer-history-records-v2/?startTime=%d&endTime=%d&pageType=VOICE_HISTORY",
		c.amazonDomain, startTime, endTime,
	)

	body := bytes.NewReader([]byte(`{"previousRequestToken": null}`))
	req, err := http.NewRequest("POST", historyURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Cookie", c.cookies)
	req.Header.Set("csrf", c.csrf)
	req.Header.Set("anti-csrftoken-a2z", c.activityCSRF)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", fmt.Sprintf("https://www.%s", c.amazonDomain))
	req.Header.Set("Referer", fmt.Sprintf("https://www.%s/alexa-privacy/apd/activity?ref=activityHistory", c.amazonDomain))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("history API error %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse the response
	var result struct {
		CustomerHistoryRecords []struct {
			RecordKey              string `json:"recordKey"`
			Timestamp              int64  `json:"timestamp"`
			VoiceHistoryRecordItems []struct {
				RecordItemType string `json:"recordItemType"`
				TranscriptText string `json:"transcriptText"`
			} `json:"voiceHistoryRecordItems"`
		} `json:"customerHistoryRecords"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse history response: %w", err)
	}

	// Convert to our simplified format
	var records []HistoryRecord
	for _, r := range result.CustomerHistoryRecords {
		record := HistoryRecord{
			RecordKey: r.RecordKey,
			Timestamp: r.Timestamp,
		}

		// Extract device from recordKey (format: customerId#timestamp#deviceType#serialNumber)
		parts := strings.Split(r.RecordKey, "#")
		if len(parts) >= 4 {
			record.Device = parts[3]
		}

		// Collect ASR (what user said) and TTS (what Alexa said)
		for _, item := range r.VoiceHistoryRecordItems {
			switch item.RecordItemType {
			case "ASR_REPLACEMENT_TEXT":
				if record.CustomerUtterance != "" {
					record.CustomerUtterance += " "
				}
				record.CustomerUtterance += item.TranscriptText
			case "TTS_REPLACEMENT_TEXT":
				if record.AlexaResponse != "" {
					record.AlexaResponse += " "
				}
				record.AlexaResponse += item.TranscriptText
			}
		}

		records = append(records, record)
	}

	return records, nil
}

// Ask sends a voice command and waits for Alexa's response
func (c *Client) Ask(device *Device, question string, timeout time.Duration) (string, error) {
	// Record the time before sending the command
	startTime := time.Now().UnixMilli() - 1000 // 1 second buffer

	// Send the command
	if err := c.SequenceCommand(device, fmt.Sprintf("textcommand:'%s'", question)); err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	// Poll for the response
	endTime := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(endTime) {
		time.Sleep(pollInterval)

		// Get recent history
		records, err := c.GetCustomerHistoryRecords(startTime, time.Now().UnixMilli()+60000)
		if err != nil {
			// Don't fail immediately, might be a transient error
			continue
		}

		// Look for a record matching our device and time
		for _, record := range records {
			// Check if this is from our device (by serial number)
			if !strings.Contains(record.RecordKey, device.SerialNumber) {
				continue
			}

			// Check if this is recent enough
			if record.Timestamp < startTime {
				continue
			}

			// Check if the utterance matches our question (case-insensitive partial match)
			if record.CustomerUtterance != "" &&
			   strings.Contains(strings.ToLower(record.CustomerUtterance), strings.ToLower(question[:min(len(question), 20)])) {
				if record.AlexaResponse != "" {
					return record.AlexaResponse, nil
				}
			}
		}
	}

	return "", fmt.Errorf("timeout waiting for Alexa response")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mustJSON(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// locale returns the appropriate locale for the user's Amazon domain
func (c *Client) locale() string {
	switch c.amazonDomain {
	case "amazon.com":
		return "en-US"
	case "amazon.co.uk":
		return "en-GB"
	case "amazon.de":
		return "de-DE"
	case "amazon.fr":
		return "fr-FR"
	case "amazon.it":
		return "it-IT"
	case "amazon.es":
		return "es-ES"
	case "amazon.co.jp":
		return "ja-JP"
	case "amazon.com.au":
		return "en-AU"
	case "amazon.ca":
		return "en-CA"
	case "amazon.com.br":
		return "pt-BR"
	case "amazon.in":
		return "en-IN"
	default:
		return "en-US"
	}
}

// ============================================================================
// Alexa+ (LLM) Support
// ============================================================================

// avsURL returns the AVS API base URL
func (c *Client) avsURL() string {
	// AVS has regional endpoints
	switch c.amazonDomain {
	case "amazon.com", "amazon.ca", "amazon.com.br":
		return "https://avs-alexa-12-na.amazon.com"
	case "amazon.co.uk", "amazon.de", "amazon.fr", "amazon.it", "amazon.es", "amazon.in":
		return "https://avs-alexa-eu.amazon.com"
	case "amazon.co.jp":
		return "https://avs-alexa-fe.amazon.com"
	default:
		// Default to EU for unknown domains (safer for most non-US users)
		return "https://avs-alexa-eu.amazon.com"
	}
}

// getBearerToken obtains an access token for AVS APIs
func (c *Client) getBearerToken() error {
	if c.bearerToken != "" {
		c.log("Using cached bearer token")
		return nil // Already have one
	}

	c.log("Requesting new bearer token from api.amazon.com/auth/token")

	authURL := "https://api.amazon.com/auth/token"

	data := url.Values{}
	data.Set("requested_token_type", "access_token")
	data.Set("source_token_type", "refresh_token")
	data.Set("source_token", c.refreshToken)
	data.Set("app_name", "Amazon Alexa")
	data.Set("app_version", "2.2.696573.0")

	req, err := http.NewRequest("POST", authURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("x-amzn-identity-auth-domain", "api.amazon.com")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bearer token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.log("Bearer token response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		c.log("Bearer token error body: %s", string(body))
		return fmt.Errorf("bearer token failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse bearer token response: %w", err)
	}

	c.bearerToken = result.AccessToken
	c.log("Got bearer token: %s...", c.bearerToken[:min(20, len(c.bearerToken))])
	return nil
}

// generateUUID generates a simple UUID-like string
func generateUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		time.Now().UnixNano()&0xFFFFFFFF,
		time.Now().UnixNano()>>32&0xFFFF,
		0x4000|time.Now().UnixNano()>>48&0x0FFF,
		0x8000|time.Now().UnixNano()>>60&0x3FFF,
		time.Now().UnixNano())
}

// CardData represents the card data inside APLFragment datasources
type CardData struct {
	Type     string `json:"type"`
	CardType string `json:"cardType"`
	Text     string `json:"text"`
	Items    []struct {
		Text  string `json:"text"`
		Style string `json:"style"`
	} `json:"items"`
}

// FragmentContent represents the content of a conversation fragment
type FragmentContent struct {
	Type     string `json:"type"`
	CardType string `json:"cardType"`
	Text     string `json:"text"`
	Items    []struct {
		Text  string `json:"text"`
		Style string `json:"style"`
	} `json:"items"`
	// For APLFragment type, text is nested in datasources.cardData
	Datasources struct {
		CardData *CardData `json:"cardData"`
	} `json:"datasources"`
}

// GetText returns the text content, handling both Card and APLFragment types
func (fc *FragmentContent) GetText() string {
	if fc.Text != "" {
		return fc.Text
	}
	if fc.Datasources.CardData != nil && fc.Datasources.CardData.Text != "" {
		return fc.Datasources.CardData.Text
	}
	return ""
}

// ConversationFragment represents an Alexa+ conversation response
type ConversationFragment struct {
	FragmentURI string           `json:"fragmentURI"`
	Timestamp   string           `json:"timestamp"`
	Content     *FragmentContent `json:"content"`
	RawContent  json.RawMessage  `json:"-"` // For debugging
	Metadata    struct {
		Purpose    string `json:"purpose"`
		Provenance struct {
			Type string `json:"type"`
		} `json:"provenance"`
	} `json:"metadata"`
}

// ConversationResponse represents the full conversation API response
type ConversationResponse struct {
	ConversationID string                 `json:"conversationId"`
	Fragments      []ConversationFragment `json:"fragments"`
	Token          string                 `json:"token"`
}

// SendAVSTextMessage sends a text message via AVS (Alexa+ Type-to-Alexa)
func (c *Client) SendAVSTextMessage(text string) error {
	_, _, err := c.SendAVSTextMessageWithResponse(text)
	return err
}

// SendAVSTextMessageWithResponse sends text and returns conversation ID and any immediate response
func (c *Client) SendAVSTextMessageWithResponse(text string) (conversationID string, responseText string, err error) {
	c.log("SendAVSTextMessageWithResponse: %q", text)

	if err := c.getBearerToken(); err != nil {
		return "", "", fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Generate unique IDs - use Mobile_TTA_ prefix like the real app
	dialogRequestID := fmt.Sprintf("Mobile_TTA_%s", strings.ToUpper(generateUUID()))
	messageID := strings.ToUpper(generateUUID())
	boundary := strings.ToUpper(generateUUID())

	c.log("Dialog request ID: %s", dialogRequestID)
	c.log("Conversation ID in context: %s", c.conversationID)

	// Build context and add the text event
	contexts := c.buildAVSContext()

	// Build the multipart form data
	event := map[string]interface{}{
		"event": map[string]interface{}{
			"header": map[string]interface{}{
				"namespace":       "Alexa.Input.Text",
				"name":            "TextMessage",
				"messageId":       messageID,
				"dialogRequestId": dialogRequestID,
			},
			"payload": map[string]interface{}{
				"text": text,
			},
		},
		"context": contexts,
	}

	eventJSON, _ := json.Marshal(event)

	// Build multipart body
	var body bytes.Buffer
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Disposition: form-data; name=\"metadata\"\r\n")
	body.WriteString("Content-Type: application/json; charset=UTF-8\r\n\r\n")
	body.Write(eventJSON)
	body.WriteString(fmt.Sprintf("\r\n--%s--", boundary))

	c.log("Request body size: %d bytes (captured was 4187)", body.Len())

	req, err2 := http.NewRequest("POST", c.avsURL()+"/v20160207/events", &body)
	if err2 != nil {
		return "", "", err2
	}

	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Alexa/2.2.696573 CFNetwork/3860.200.71 Darwin/25.1.0")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Priority", "u=1, i")
	// Add cookies - the AVS endpoint may need session cookies in addition to bearer token
	if c.cookies != "" {
		req.Header.Set("Cookie", c.cookies)
	}

	resp, err2 := c.httpClient.Do(req)
	if err2 != nil {
		return "", "", fmt.Errorf("AVS request failed: %w", err2)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	c.log("AVS response status: %d", resp.StatusCode)
	c.log("AVS response length: %d bytes", len(respBody))

	// 204 No Content is success but no data
	if resp.StatusCode == http.StatusNoContent {
		c.log("Got 204 No Content - no response data")
		return "", "", nil
	}

	if resp.StatusCode != http.StatusOK {
		c.log("AVS error response: %s", string(respBody))
		return "", "", fmt.Errorf("AVS error %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse multipart response for AddFragments directives
	// Response format: multipart/related with JSON directives
	respStr := string(respBody)

	c.log("AVS response (first 500 chars): %s", respStr[:min(500, len(respStr))])

	// Extract conversation ID from AddFragments directive
	convIDRegex := regexp.MustCompile(`"conversationId"\s*:\s*"(amzn1\.conversation\.[^"]+)"`)
	if matches := convIDRegex.FindStringSubmatch(respStr); len(matches) > 1 {
		conversationID = matches[1]
		c.log("Found conversation ID: %s", conversationID)
	}

	// Look for LLM response text in fragments
	textRegex := regexp.MustCompile(`"text"\s*:\s*"([^"]+)"`)
	if matches := textRegex.FindAllStringSubmatch(respStr, -1); len(matches) > 0 {
		c.log("Found %d text matches in response", len(matches))
		// Skip the first match which is usually the user's question
		for i, match := range matches {
			if i > 0 && len(match) > 1 && !strings.Contains(match[1], text) {
				// This might be an LLM response
				if strings.Contains(respStr, "LLM:APE") || strings.Contains(respStr, `"purpose":"AGENT"`) {
					responseText = match[1]
					c.log("Found potential LLM response: %s", responseText)
					break
				}
			}
		}
	}

	return conversationID, responseText, nil
}

// buildAVSContext returns the full context array needed for AVS requests
func (c *Client) buildAVSContext() []map[string]interface{} {
	contexts := []map[string]interface{}{
		{
			"header": map[string]interface{}{
				"namespace": "SpeechSynthesizer",
				"name":      "SpeechState",
			},
			"payload": map[string]interface{}{
				"playerActivity":       "FINISHED",
				"token":                "",
				"offsetInMilliseconds": 0,
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "SpeechRecognizer",
				"name":      "RecognizerState",
			},
			"payload": map[string]interface{}{
				"wakeword": "ALEXA",
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Speaker",
				"name":      "VolumeState",
			},
			"payload": map[string]interface{}{
				"volume": 50,
				"muted":  false,
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Alexa.Display.Window",
				"name":      "WindowState",
			},
			"payload": map[string]interface{}{
				"defaultWindowId": "app_window",
				"instances": []map[string]interface{}{
					{
						"id":         "app_window",
						"templateId": "app_window_template",
						"configuration": map[string]interface{}{
							"interactionMode":     "mobile_mode",
							"sizeConfigurationId": "fullscreen",
						},
					},
				},
			},
		},
		{
			// Critical: Tell Alexa this is a text-focused interaction
			"header": map[string]interface{}{
				"namespace": "VisualActivityTracker",
				"name":      "ActivityState",
			},
			"payload": map[string]interface{}{
				"focused": map[string]interface{}{
					"interface": "Text",
				},
			},
		},
		{
			// Indicate audio is idle
			"header": map[string]interface{}{
				"namespace": "AudioActivityTracker",
				"name":      "ActivityState",
			},
			"payload": map[string]interface{}{
				"dialog": map[string]interface{}{
					"interface":            "SpeechSynthesizer",
					"idleTimeInMilliseconds": 100000,
				},
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Alerts",
				"name":      "AlertsState",
			},
			"payload": map[string]interface{}{
				"allAlerts":    []interface{}{},
				"activeAlerts": []interface{}{},
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Alexa.IOComponents",
				"name":      "TrustedStates",
			},
			"payload": map[string]interface{}{
				"sessionStates": []interface{}{},
				"unlockState":   "NEVER_UNLOCKED",
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Alexa.IOComponents",
				"name":      "IOComponentStates",
			},
			"payload": map[string]interface{}{
				"activeIOComponents": []interface{}{},
				"allIOComponents":    []interface{}{},
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Alexa.PlaybackStateReporter",
				"name":      "PlaybackState",
			},
			"payload": map[string]interface{}{
				"state":               "IDLE",
				"shuffle":             "NOT_SHUFFLED",
				"repeat":              "NOT_REPEATED",
				"favorite":            "NOT_RATED",
				"positionMilliseconds": 0,
				"supportedOperations": []string{"Play", "Pause", "Previous", "Next"},
				"players":             []interface{}{},
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Alexa.IOComponents.Bluetooth",
				"name":      "BluetoothState",
			},
			"payload": map[string]interface{}{
				"bluetoothStates": []interface{}{},
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Alexa.Identity.Recognition",
				"name":      "RecognitionState",
			},
			"payload": map[string]interface{}{
				"RecognitionState": map[string]interface{}{
					"primaryPerson": map[string]interface{}{
						"acl": 100,
						"id":  "amzn1.actor.person.did.AP4LASCN2HNWAAMTI32QX37S6QDPIFEQ2UGPIFWPF54F3Y7WDU4V4X273UV3BSP7ENUDDTIJ",
					},
				},
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "ExternalMediaPlayer",
				"name":      "ExternalMediaPlayerState",
			},
			"payload": map[string]interface{}{
				"agent":         "XOGFXO466L",
				"spiVersion":    "2.2.0",
				"players":       []interface{}{},
				"playerInFocus": "",
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Alexa.Comms.PhoneCallController",
				"name":      "PhoneCallControllerState",
			},
			"payload": map[string]interface{}{
				"allCalls":    []interface{}{},
				"currentCall": map[string]interface{}{},
				"device": map[string]interface{}{
					"connectionState": "DISCONNECTED",
				},
				"configuration": map[string]interface{}{
					"callingFeature": []map[string]string{
						{"OVERRIDE_RINGTONE_SUPPORTED": "false"},
					},
				},
			},
		},
		{
			"header": map[string]interface{}{
				"namespace": "Alexa.Comms.MessagingController",
				"name":      "MessagingControllerState",
			},
			"payload": map[string]interface{}{
				"messagingEndpointStates": []map[string]interface{}{
					{
						"messagingEndpointInfo": map[string]string{"name": "DEFAULT"},
						"permissions": map[string]string{
							"sendPermission": "OFF",
							"readPermission": "OFF",
						},
						"connectionState": "DISCONNECTED",
					},
				},
			},
		},
	}

	// Add conversation context
	convPayload := map[string]interface{}{
		"type":        "VCF2",
		"version":     "2024.1",
		"windowState": "NORMAL",
		"size": map[string]interface{}{
			"width":  430,
			"height": 932,
		},
		"scrollable": map[string]interface{}{
			"direction":    "vertical",
			"allowForward": false,
			"allowBackward": true,
		},
		"elements": []interface{}{},
	}
	if c.conversationID != "" {
		convPayload["conversationId"] = c.conversationID
	}
	contexts = append(contexts, map[string]interface{}{
		"header": map[string]interface{}{
			"namespace": "Alexa.Conversation",
			"name":      "ConversationState",
		},
		"payload": convPayload,
	})

	return contexts
}

// GetConversationFragments retrieves the latest conversation fragments
func (c *Client) GetConversationFragments() (*ConversationResponse, error) {
	if c.conversationID == "" {
		return nil, fmt.Errorf("no conversation ID set")
	}

	if err := c.getBearerToken(); err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	// Build URL with token if available
	fragURL := fmt.Sprintf("%s/v1/conversations/%s/fragments/synchronize",
		c.avsURL(), c.conversationID)

	req, err := http.NewRequest("GET", fragURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	req.Header.Set("Cookie", c.cookies)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conversation API error %d: %s", resp.StatusCode, string(body))
	}

	var result ConversationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse conversation response: %w", err)
	}

	// Debug: show fragment summary
	if c.verbose {
		agentCount := 0
		for _, frag := range result.Fragments {
			if frag.Metadata.Purpose == "AGENT" && frag.Content != nil {
				text := frag.Content.GetText()
				if text != "" {
					agentCount++
				}
			}
		}
		c.log("Fragments: %d total, %d AGENT with text", len(result.Fragments), agentCount)
	}

	return &result, nil
}

// InitConversation creates or retrieves a conversation ID
func (c *Client) InitConversation() error {
	if c.conversationID != "" {
		return nil
	}

	// Generate a new conversation ID in Amazon's format
	c.conversationID = fmt.Sprintf("amzn1.conversation.%s", generateUUID())
	return nil
}

// SetConversationID sets an existing conversation ID
func (c *Client) SetConversationID(id string) {
	c.conversationID = id
}

// SynchronizeState sends a state sync event to AVS (initializes session)
func (c *Client) SynchronizeState() error {
	if err := c.getBearerToken(); err != nil {
		return fmt.Errorf("failed to get bearer token: %w", err)
	}

	messageID := strings.ToUpper(generateUUID())
	boundary := strings.ToUpper(generateUUID())

	event := map[string]interface{}{
		"event": map[string]interface{}{
			"header": map[string]interface{}{
				"namespace": "System",
				"name":      "SynchronizeState",
				"messageId": messageID,
			},
			"payload": map[string]interface{}{},
		},
		"context": c.buildAVSContext(),
	}

	eventJSON, _ := json.Marshal(event)

	var body bytes.Buffer
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Disposition: form-data; name=\"metadata\"\r\n")
	body.WriteString("Content-Type: application/json; charset=UTF-8\r\n\r\n")
	body.Write(eventJSON)
	body.WriteString(fmt.Sprintf("\r\n--%s--", boundary))

	c.log("SynchronizeState request size: %d bytes", body.Len())

	req, err := http.NewRequest("POST", c.avsURL()+"/v20160207/events", &body)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Alexa/2.2.696573 CFNetwork/3860.200.71 Darwin/25.1.0")
	req.Header.Set("Priority", "u=3")
	if c.cookies != "" {
		req.Header.Set("Cookie", c.cookies)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("SynchronizeState request failed: %w", err)
	}
	defer resp.Body.Close()

	c.log("SynchronizeState response status: %d", resp.StatusCode)

	// 204 No Content is expected for SynchronizeState
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SynchronizeState error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Conversation represents an Alexa+ conversation with a device
type Conversation struct {
	ConversationID string `json:"conversationId"`
	DeviceName     string `json:"deviceName"`
	LastTurnTime   string `json:"lastTurnTime,omitempty"`
}

// GetConversations retrieves all Alexa+ conversations and their associated devices
func (c *Client) GetConversations() ([]Conversation, error) {
	if err := c.getBearerToken(); err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	req, err := http.NewRequest("GET", c.avsURL()+"/v1/conversations", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	req.Header.Set("Accept", "application/json")
	if c.cookies != "" {
		req.Header.Set("Cookie", c.cookies)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conversations API error %d: %s", resp.StatusCode, string(body))
	}

	// Response structure: { "conversations": [ { "id": "...", "creation": { "origin": { "name": "..." } } }, ... ] }
	var result struct {
		Conversations []struct {
			ID       string `json:"id"`
			Creation struct {
				Origin struct {
					Name string `json:"name"`
				} `json:"origin"`
				Time string `json:"time"`
			} `json:"creation"`
			LastTurn struct {
				Origin struct {
					Name string `json:"name"`
				} `json:"origin"`
				Time string `json:"time"`
			} `json:"lastTurn"`
		} `json:"conversations"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse conversations response: %w", err)
	}

	var conversations []Conversation
	for _, item := range result.Conversations {
		conv := Conversation{
			ConversationID: item.ID,
			DeviceName:     item.Creation.Origin.Name,
			LastTurnTime:   item.LastTurn.Time,
		}
		// Prefer lastTurn device name if available (more recent)
		if item.LastTurn.Origin.Name != "" {
			conv.DeviceName = item.LastTurn.Origin.Name
		}
		// Fall back to creation time if no last turn
		if conv.LastTurnTime == "" {
			conv.LastTurnTime = item.Creation.Time
		}
		conversations = append(conversations, conv)
	}

	return conversations, nil
}

// GetConversationForDevice finds the most recent conversation ID for a device name
// Returns empty string if no conversation found
func (c *Client) GetConversationForDevice(deviceName string) (string, error) {
	conversations, err := c.GetConversations()
	if err != nil {
		return "", err
	}

	deviceLower := strings.ToLower(deviceName)
	var bestMatch *Conversation

	for i := range conversations {
		conv := &conversations[i]
		if strings.Contains(strings.ToLower(conv.DeviceName), deviceLower) {
			// First match or more recent than current best
			if bestMatch == nil || conv.LastTurnTime > bestMatch.LastTurnTime {
				bestMatch = conv
			}
		}
	}

	if bestMatch == nil {
		return "", fmt.Errorf("no Alexa+ conversation found for device %q", deviceName)
	}

	return bestMatch.ConversationID, nil
}

// AskPlus sends a question via Alexa+ (LLM) and returns the response
func (c *Client) AskPlus(question string, timeout time.Duration) (string, error) {
	c.log("AskPlus: %q (timeout: %v)", question, timeout)

	// First, sync state with AVS
	c.log("Sending SynchronizeState...")
	if err := c.SynchronizeState(); err != nil {
		c.log("SynchronizeState error (continuing anyway): %v", err)
	}

	// Send the text message and get conversation ID from response
	convID, initialText, err := c.SendAVSTextMessageWithResponse(question)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	// If we got a direct response, return it
	if initialText != "" {
		c.log("Got direct response from AVS POST")
		return initialText, nil
	}

	// Set conversation ID for polling
	if convID != "" {
		c.conversationID = convID
		c.log("Set conversation ID for polling: %s", convID)
	}

	// If no conversation ID, we can't poll
	if c.conversationID == "" {
		return "", fmt.Errorf("no conversation ID received from Alexa")
	}

	// Poll for new fragments
	endTime := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond
	pollCount := 0

	c.log("Starting to poll conversation fragments...")

	for time.Now().Before(endTime) {
		time.Sleep(pollInterval)
		pollCount++

		resp, err := c.GetConversationFragments()
		if err != nil {
			c.log("Poll %d error: %v", pollCount, err)
			continue // Keep trying
		}

		c.log("Poll %d: got %d fragments", pollCount, len(resp.Fragments))

		// Look for AGENT responses with text content
		for i, frag := range resp.Fragments {
			hasContent := frag.Content != nil
			text := ""
			if hasContent {
				text = frag.Content.GetText()
			}
			hasText := text != ""
			if pollCount == 1 && i < 5 {
				// Dump first 5 fragments on first poll for debugging
				c.log("Frag[%d]: purpose=%s, uri=%s, hasContent=%v, hasText=%v",
					i, frag.Metadata.Purpose, frag.FragmentURI, hasContent, hasText)
			}
			if hasText {
				c.log("Fragment with text: purpose=%s, uri=%s, text=%q",
					frag.Metadata.Purpose, frag.FragmentURI, text[:min(50, len(text))])

				if frag.Metadata.Purpose == "AGENT" ||
					strings.Contains(frag.FragmentURI, "LLM:APE") {
					// Build response with source citations if present
					result := text
					// Check Items from either direct content or cardData
					var items []struct {
						Text  string `json:"text"`
						Style string `json:"style"`
					}
					if len(frag.Content.Items) > 0 {
						items = frag.Content.Items
					} else if frag.Content.Datasources.CardData != nil {
						items = frag.Content.Datasources.CardData.Items
					}
					for _, item := range items {
						if item.Style == "text-style-attribution" {
							result += "\n" + item.Text
						}
					}
					return result, nil
				}
			}
		}
	}

	return "", fmt.Errorf("timeout waiting for Alexa+ response")
}
