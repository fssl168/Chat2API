package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/fssl168/chat2api-go/oauth/pkg/oauth"
	"github.com/fssl168/chat2api-go/oauth/pkg/oauth/adapter"
)

const (
	defaultPort  = "8080"
	dataDir      = ".chat2api"
	accountsFile = "accounts.json"
)

type BrowserTask struct {
	ID           string             `json:"id"`
	Status       string             `json:"status"`
	ProviderID   string             `json:"providerId"`
	ProviderType string             `json:"providerType"`
	Result       *oauth.OAuthResult `json:"result,omitempty"`
	Error        string             `json:"error,omitempty"`
	Logs         []oauth.LogEntry   `json:"logs,omitempty"`
	CreatedAt    time.Time          `json:"createdAt"`
	UpdatedAt    time.Time          `json:"updatedAt"`
	mu           sync.RWMutex
}

type StoredAccount struct {
	ID            string            `json:"id"`
	ProviderID    string            `json:"providerId"`
	ProviderType  string            `json:"providerType"`
	Name          string            `json:"name"`
	Credentials   map[string]string `json:"credentials"`
	Status        string            `json:"status"`
	AccountInfo   *AccountInfo      `json:"accountInfo,omitempty"`
	DailyLimit    int               `json:"dailyLimit,omitempty"`
	TodayUsed     int               `json:"todayUsed,omitempty"`
	RequestCount  int               `json:"requestCount,omitempty"`
	ErrorMessage  string            `json:"errorMessage,omitempty"`
	LastUsed      string            `json:"lastUsed,omitempty"`
	LastValidated string            `json:"lastValidated,omitempty"`
	CreatedAt     string            `json:"createdAt"`
}

type AccountInfo struct {
	UserID string `json:"userId,omitempty"`
	Email  string `json:"email,omitempty"`
	Name   string `json:"name,omitempty"`
}

type CredentialField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	HelpText    string `json:"helpText,omitempty"`
}

type ProviderConfig struct {
	ID               string            `json:"id"`
	ProviderType     string            `json:"providerType"`
	Label            string            `json:"label"`
	Description      string            `json:"description"`
	HelpURL          string            `json:"helpUrl,omitempty"`
	LoginURL         string            `json:"loginURL"`
	SupportedModels  []string          `json:"supportedModels,omitempty"`
	AuthType         string            `json:"authType"`
	CredentialFields []CredentialField `json:"credentialFields"`
	Enabled          bool              `json:"enabled"`
}

type AccountStore struct {
	accounts map[string][]*StoredAccount
	mu       sync.RWMutex
	filePath string
}

func NewAccountStore() *AccountStore {
	homeDir, _ := os.UserHomeDir()
	dir := filepath.Join(homeDir, dataDir)
	os.MkdirAll(dir, 0755)
	fp := filepath.Join(dir, accountsFile)

	store := &AccountStore{
		accounts: make(map[string][]*StoredAccount),
		filePath: fp,
	}
	store.load()
	return store
}

func (s *AccountStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.accounts)
}

func (s *AccountStore) save() {
	data, err := json.MarshalIndent(s.accounts, "", "  ")
	if err != nil {
		log.Printf("[AccountStore] Failed to marshal: %v", err)
		return
	}
	os.WriteFile(s.filePath, data, 0644)
}

func (s *AccountStore) GetAccounts(providerType string) []*StoredAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	accounts := s.accounts[providerType]
	if accounts == nil {
		return []*StoredAccount{}
	}
	result := make([]*StoredAccount, len(accounts))
	copy(result, accounts)
	return result
}

func (s *AccountStore) GetAccount(providerType, accountID string) *StoredAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.accounts[providerType] {
		if a.ID == accountID {
			return a
		}
	}
	return nil
}

func (s *AccountStore) AddAccount(providerType string, account *StoredAccount) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[providerType] = append(s.accounts[providerType], account)
	s.save()
}

func (s *AccountStore) UpdateAccount(providerType, accountID string, updates map[string]interface{}) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.accounts[providerType] {
		if a.ID == accountID {
			if v, ok := updates["name"].(string); ok {
				a.Name = v
			}
			if v, ok := updates["credentials"].(map[string]string); ok {
				a.Credentials = v
			}
			if v, ok := updates["status"].(string); ok {
				a.Status = v
			}
			if v, ok := updates["errorMessage"].(string); ok {
				a.ErrorMessage = v
			}
			if v, ok := updates["dailyLimit"].(float64); ok {
				a.DailyLimit = int(v)
			}
			if v, ok := updates["lastValidated"].(string); ok {
				a.LastValidated = v
			}
			if v, ok := updates["lastUsed"].(string); ok {
				a.LastUsed = v
			}
			if v, ok := updates["accountInfo"].(map[string]interface{}); ok {
				if a.AccountInfo == nil {
					a.AccountInfo = &AccountInfo{}
				}
				if n, ok := v["name"].(string); ok {
					a.AccountInfo.Name = n
				}
				if e, ok := v["email"].(string); ok {
					a.AccountInfo.Email = e
				}
				if u, ok := v["userId"].(string); ok {
					a.AccountInfo.UserID = u
				}
			}
			s.accounts[providerType][i] = a
			s.save()
			return true
		}
	}
	return false
}

func (s *AccountStore) DeleteAccount(providerType, accountID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	accounts := s.accounts[providerType]
	for i, a := range accounts {
		if a.ID == accountID {
			s.accounts[providerType] = append(accounts[:i], accounts[i+1:]...)
			s.save()
			return true
		}
	}
	return false
}

type Server struct {
	engine       *gin.Engine
	manager      *adapter.OAuthManager
	port         string
	tasks        map[string]*BrowserTask
	taskMu       sync.RWMutex
	accountStore *AccountStore
	providers    []ProviderConfig
}

func NewServer(port string) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.Default()

	manager := adapter.NewOAuthManager()
	manager.SetProgressCallback(func(event oauth.OAuthProgressEvent) {
		log.Printf("[OAuth Progress] %s: %s\n", event.Status, event.Message)
	})

	providers := buildProviderConfigs()

	return &Server{
		engine:       engine,
		manager:      manager,
		port:         port,
		tasks:        make(map[string]*BrowserTask),
		accountStore: NewAccountStore(),
		providers:    providers,
	}
}

func buildProviderConfigs() []ProviderConfig {
	return []ProviderConfig{
		{
			ID: "deepseek", ProviderType: "deepseek", Label: "DeepSeek",
			Description: "DeepSeek AI assistant, supports deep thinking and web search",
			HelpURL:     "https://chat.deepseek.com", LoginURL: "https://chat.deepseek.com",
			SupportedModels: []string{
				"deepseek-v4-pro", "deepseek-v4-pro-think", "deepseek-v4-pro-search", "deepseek-v4-pro-think-search",
				"deepseek-v4-flash", "deepseek-v4-flash-think", "deepseek-v4-flash-search", "deepseek-v4-flash-think-search",
				"deepseek-v4-vision", "deepseek-chat", "deepseek-reasoner",
				"DeepSeek-Search", "DeepSeek-R1", "DeepSeek-R1-Search",
			},
			AuthType: "userToken",
			CredentialFields: []CredentialField{
				{Name: "token", Label: "User Token", Type: "password", Required: true, Placeholder: "Enter DeepSeek User Token", HelpText: "From DeepSeek web version, DevTools Application -> Local Storage"},
			},
			Enabled: true,
		},
		{
			ID: "glm", ProviderType: "glm", Label: "GLM",
			Description: "Zhipu Qingyan AI assistant, supports GLM-5 flagship model",
			HelpURL:     "https://chatglm.cn", LoginURL: "https://chatglm.cn",
			SupportedModels: []string{"GLM-5"},
			AuthType:        "refresh_token",
			CredentialFields: []CredentialField{
				{Name: "refresh_token", Label: "Refresh Token", Type: "password", Required: true, Placeholder: "Enter GLM Refresh Token", HelpText: "From chatglm.cn, DevTools Application -> Local Storage -> chatglm_refresh_token"},
			},
			Enabled: true,
		},
		{
			ID: "kimi", ProviderType: "kimi", Label: "Kimi",
			Description: "Kimi K2.6 AI assistant by Moonshot, supports thinking mode and web search",
			HelpURL:     "https://www.kimi.com", LoginURL: "https://www.kimi.com",
			SupportedModels: []string{"Kimi-K2.6", "Kimi-K2.5"},
			AuthType:        "jwt",
			CredentialFields: []CredentialField{
				{Name: "token", Label: "Access Token (JWT)", Type: "password", Required: true, Placeholder: "Enter Kimi JWT Token", HelpText: "From kimi.com Authorization header or kimi-auth cookie"},
			},
			Enabled: true,
		},
		{
			ID: "minimax", ProviderType: "minimax", Label: "MiniMax",
			Description: "MiniMax Agent - AI assistant with MCP multi-agent collaboration",
			HelpURL:     "https://agent.minimaxi.com", LoginURL: "https://agent.minimaxi.com",
			SupportedModels: []string{"MiniMax-M2.5", "MiniMax-M2.7"},
			AuthType:        "jwt",
			CredentialFields: []CredentialField{
				{Name: "token", Label: "JWT Token", Type: "password", Required: true, Placeholder: "Enter MiniMax JWT Token", HelpText: "Format: realUserID+JWTtoken or just JWT token"},
				{Name: "realUserID", Label: "Real User ID", Type: "text", Required: false, Placeholder: "Optional: realUserID"},
			},
			Enabled: true,
		},
		{
			ID: "qwen", ProviderType: "qwen", Label: "Qwen",
			Description: "Qwen AI assistant by Alibaba Cloud (www.qianwen.com)",
			HelpURL:     "https://www.qianwen.com", LoginURL: "https://tongyi.aliyun.com",
			SupportedModels: []string{"Qwen3", "Qwen3-Max", "Qwen3-Max-Thinking", "Qwen3-Plus", "Qwen3.5-Plus", "Qwen3-Flash", "Qwen3-Coder"},
			AuthType:        "tongyi_sso_ticket",
			CredentialFields: []CredentialField{
				{Name: "ticket", Label: "SSO Ticket", Type: "password", Required: true, Placeholder: "Enter tongyi_sso_ticket", HelpText: "From www.qianwen.com, DevTools Application -> Cookies -> tongyi_sso_ticket"},
			},
			Enabled: true,
		},
		{
			ID: "qwen-ai", ProviderType: "qwen-ai", Label: "Qwen AI",
			Description: "Qwen AI international version (chat.qwen.ai)",
			HelpURL:     "https://chat.qwen.ai", LoginURL: "https://chat.qwen.ai",
			SupportedModels: []string{
				"Qwen3.6-Plus", "Qwen3.5-Plus", "Qwen3.5-Omni-Plus", "Qwen3.5-Flash",
				"Qwen3.5-Max-Preview", "Qwen3.6-Plus-Preview",
				"Qwen3.5-397B-A17B", "Qwen3.5-122B-A10B", "Qwen3.5-Omni-Flash",
				"Qwen3.5-27B", "Qwen3.5-35B-A3B",
				"Qwen3-Max", "Qwen3-235B-A22B-2507", "Qwen3-Coder",
				"Qwen3-VL-235B-A22B", "Qwen3-Omni-Flash", "Qwen2.5-Max",
			},
			AuthType: "jwt",
			CredentialFields: []CredentialField{
				{Name: "token", Label: "Auth Token", Type: "password", Required: true, Placeholder: "Enter Qwen AI Token", HelpText: "From chat.qwen.ai Local Storage (key: token)"},
				{Name: "cookies", Label: "Cookies (Optional)", Type: "textarea", Required: false, Placeholder: "Optional: full cookie string"},
			},
			Enabled: true,
		},
		{
			ID: "zai", ProviderType: "zai", Label: "Z.ai",
			Description: "Z.ai - Free AI Chatbot powered by GLM-5 and GLM-4.7",
			HelpURL:     "https://chat.z.ai", LoginURL: "https://chat.z.ai",
			SupportedModels: []string{"GLM-5-Turbo", "glm-5", "glm-4.7", "glm-4.6v", "glm-4.6", "glm-4.5v", "glm-4.5-air"},
			AuthType:        "jwt",
			CredentialFields: []CredentialField{
				{Name: "token", Label: "Access Token", Type: "password", Required: true, Placeholder: "Enter Z.ai Token", HelpText: "From chat.z.ai, DevTools Application -> Cookie, starts with eyJ..."},
			},
			Enabled: true,
		},
		{
			ID: "perplexity", ProviderType: "perplexity", Label: "Perplexity",
			Description: "Perplexity AI search assistant with multi-model support and web search",
			HelpURL:     "https://www.perplexity.ai", LoginURL: "https://www.perplexity.ai",
			SupportedModels: []string{"Auto", "Turbo", "PPLX-Pro", "GPT-5", "Gemini-2.5-Pro", "Claude-Sonnet-4", "Claude-Opus-4", "Nemotron"},
			AuthType:        "cookie",
			CredentialFields: []CredentialField{
				{Name: "sessionToken", Label: "Session Token", Type: "password", Required: true, Placeholder: "Enter Perplexity Session Token", HelpText: "From perplexity.ai, __Secure-next-auth.session-token cookie"},
			},
			Enabled: true,
		},
		{
			ID: "mimo", ProviderType: "mimo", Label: "Mimo",
			Description: "XiaomiMIMO - Xiaomi General Intelligence Foundation Model",
			HelpURL:     "https://aistudio.xiaomimimo.com", LoginURL: "https://aistudio.xiaomimimo.com",
			SupportedModels: []string{"MiMo-V2.5-Pro", "MiMo-V2.5", "MiMo-V2-Flash"},
			AuthType:        "cookie",
			CredentialFields: []CredentialField{
				{Name: "service_token", Label: "Service Token", Type: "password", Required: true, Placeholder: "Enter Mimo Service Token", HelpText: "From DevTools -> Application -> Cookies -> serviceToken"},
				{Name: "user_id", Label: "User ID", Type: "text", Required: true, Placeholder: "Enter User ID", HelpText: "From DevTools -> Application -> Cookies -> userId"},
				{Name: "ph_token", Label: "PH Token", Type: "password", Required: true, Placeholder: "Enter PH Token", HelpText: "From DevTools -> Application -> Cookies -> xiaomichatbot_ph"},
			},
			Enabled: true,
		},
	}
}

func (s *Server) setupRoutes() {
	s.engine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	api := s.engine.Group("/api")
	{
		v1 := api.Group("/v1")
		{
			v1.GET("/providers", s.getProviders)

			providersGroup := v1.Group("/providers")
			{
				providersGroup.GET("/:providerType/accounts", s.listAccounts)
				providersGroup.POST("/:providerType/accounts", s.createAccount)
				providersGroup.GET("/:providerType/accounts/:accountId", s.getAccount)
				providersGroup.PUT("/:providerType/accounts/:accountId", s.updateAccount)
				providersGroup.DELETE("/:providerType/accounts/:accountId", s.deleteAccount)
				providersGroup.POST("/:providerType/accounts/:accountId/validate", s.validateAccount)
			}

			oauthGroup := v1.Group("/oauth")
			{
				oauthGroup.GET("/providers", s.getProviders)
				oauthGroup.POST("/login", s.startLogin)
				oauthGroup.POST("/login/token", s.loginWithToken)
				oauthGroup.POST("/login/browser", s.loginWithBrowser)
				oauthGroup.GET("/browser/tasks/:taskId", s.getBrowserTaskStatus)
				oauthGroup.POST("/validate", s.validateToken)
				oauthGroup.POST("/refresh", s.refreshToken)
				oauthGroup.GET("/callback", s.handleCallback)
			}
		}
	}

	s.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "timestamp": time.Now().Unix()})
	})
}

func (s *Server) getProviders(c *gin.Context) {
	result := make([]ProviderConfig, len(s.providers))
	copy(result, s.providers)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func (s *Server) listAccounts(c *gin.Context) {
	providerType := c.Param("providerType")
	accounts := s.accountStore.GetAccounts(providerType)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": accounts})
}

func (s *Server) getAccount(c *gin.Context) {
	providerType := c.Param("providerType")
	accountID := c.Param("accountId")
	account := s.accountStore.GetAccount(providerType, accountID)
	if account == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Account not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": account})
}

func (s *Server) createAccount(c *gin.Context) {
	providerType := c.Param("providerType")
	var account StoredAccount
	if err := c.ShouldBindJSON(&account); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if account.ID == "" {
		account.ID = fmt.Sprintf("%s-%d", providerType, time.Now().UnixMilli())
	}
	account.ProviderType = providerType
	if account.ProviderID == "" {
		account.ProviderID = providerType
	}
	if account.Status == "" {
		account.Status = "inactive"
	}
	if account.CreatedAt == "" {
		account.CreatedAt = time.Now().Format(time.RFC3339)
	}

	s.accountStore.AddAccount(providerType, &account)
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": account})
}

func (s *Server) updateAccount(c *gin.Context) {
	providerType := c.Param("providerType")
	accountID := c.Param("accountId")

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	if s.accountStore.UpdateAccount(providerType, accountID, updates) {
		account := s.accountStore.GetAccount(providerType, accountID)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": account})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Account not found"})
	}
}

func (s *Server) deleteAccount(c *gin.Context) {
	providerType := c.Param("providerType")
	accountID := c.Param("accountId")

	if s.accountStore.DeleteAccount(providerType, accountID) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Account not found"})
	}
}

func (s *Server) validateAccount(c *gin.Context) {
	providerType := c.Param("providerType")
	accountID := c.Param("accountId")

	account := s.accountStore.GetAccount(providerType, accountID)
	if account == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Account not found"})
		return
	}

	result, err := s.manager.ValidateToken(account.ProviderID, oauth.ProviderType(account.ProviderType), account.Credentials)
	if err != nil {
		s.accountStore.UpdateAccount(providerType, accountID, map[string]interface{}{
			"status":       "error",
			"errorMessage": err.Error(),
		})
		c.JSON(http.StatusOK, gin.H{"success": false, "data": gin.H{"valid": false, "error": err.Error()}})
		return
	}

	updates := map[string]interface{}{
		"status":        "active",
		"lastValidated": time.Now().Format(time.RFC3339),
	}
	if !result.Valid {
		updates["status"] = "error"
		updates["errorMessage"] = result.Error
	}
	if result.AccountInfo != nil {
		updates["accountInfo"] = map[string]string{
			"userId": result.AccountInfo.UserID,
			"email":  result.AccountInfo.Email,
			"name":   result.AccountInfo.Name,
		}
		if result.AccountInfo.Name != "" && account.Name == "" {
			updates["name"] = result.AccountInfo.Name
		}
	}
	s.accountStore.UpdateAccount(providerType, accountID, updates)

	updated := s.accountStore.GetAccount(providerType, accountID)
	c.JSON(http.StatusOK, gin.H{"success": result.Valid, "data": result, "account": updated})
}

func (s *Server) startLogin(c *gin.Context) {
	var req struct {
		ProviderID   string `json:"providerId" binding:"required"`
		ProviderType string `json:"providerType" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	result, err := s.manager.StartLogin(oauth.OAuthOptions{
		ProviderID:   req.ProviderID,
		ProviderType: oauth.ProviderType(req.ProviderType),
		Timeout:      300000,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": result.Success,
		"data":    result,
	})
}

func (s *Server) loginWithToken(c *gin.Context) {
	var req struct {
		ProviderID   string            `json:"providerId" binding:"required"`
		ProviderType string            `json:"providerType" binding:"required"`
		Token        string            `json:"token" binding:"required"`
		Extras       map[string]string `json:"extras,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	var extraSlice []string
	if req.Extras != nil {
		for _, v := range req.Extras {
			extraSlice = append(extraSlice, v)
		}
	}

	result, err := s.manager.LoginWithToken(
		req.ProviderID,
		oauth.ProviderType(req.ProviderType),
		req.Token,
		extraSlice...,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": result.Success,
		"data":    result,
	})
}

func (s *Server) loginWithBrowser(c *gin.Context) {
	var req struct {
		ProviderID   string `json:"providerId" binding:"required"`
		ProviderType string `json:"providerType" binding:"required"`
		Timeout      int    `json:"timeout,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	taskID := oauth.GenerateUUID()
	timeout := 300
	if req.Timeout > 0 && req.Timeout < 10000 {
		timeout = req.Timeout
	} else if req.Timeout >= 10000 {
		timeout = req.Timeout / 1000
	}

	task := &BrowserTask{
		ID:           taskID,
		Status:       "pending",
		ProviderID:   req.ProviderID,
		ProviderType: req.ProviderType,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	s.taskMu.Lock()
	s.tasks[taskID] = task
	s.taskMu.Unlock()

	go func() {
		log.Printf("[Browser Task %s] Starting browser automation for %s...", taskID, req.ProviderType)

		task.mu.Lock()
		task.Status = "running"
		task.UpdatedAt = time.Now()
		task.mu.Unlock()

		result, flowLog, err := s.manager.StartLoginWithBrowserAndLogs(oauth.OAuthOptions{
			ProviderID:   req.ProviderID,
			ProviderType: oauth.ProviderType(req.ProviderType),
			Timeout:      timeout,
		})

		task.mu.Lock()
		defer task.mu.Unlock()

		task.UpdatedAt = time.Now()

		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
			log.Printf("[Browser Task %s] Failed: %v", taskID, err)
		} else if result.Success {
			task.Status = "completed"
			task.Result = &result
			log.Printf("[Browser Task %s] Completed successfully!", taskID)
		} else {
			task.Status = "failed"
			task.Error = result.Error
			task.Result = &result
			log.Printf("[Browser Task %s] Failed: %s", taskID, result.Error)
		}

		if flowLog != nil {
			task.Logs = flowLog.GetEntries()
			log.Printf("[Browser Task %s] Captured %d log entries", taskID, len(task.Logs))
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"data": gin.H{
			"taskId":       taskID,
			"status":       "pending",
			"message":      "Browser automation started. Poll /api/v1/oauth/browser/tasks/:taskId for status.",
			"pollUrl":      "/api/v1/oauth/browser/tasks/" + taskID,
			"providerType": req.ProviderType,
		},
	})
}

func (s *Server) getBrowserTaskStatus(c *gin.Context) {
	taskID := c.Param("taskId")

	s.taskMu.RLock()
	task, exists := s.tasks[taskID]
	s.taskMu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Task not found: " + taskID,
		})
		return
	}

	task.mu.RLock()
	defer task.mu.RUnlock()

	response := gin.H{
		"taskId":       task.ID,
		"status":       task.Status,
		"providerId":   task.ProviderID,
		"providerType": task.ProviderType,
		"createdAt":    task.CreatedAt.Format(time.RFC3339),
		"updatedAt":    task.UpdatedAt.Format(time.RFC3339),
		"logs":         task.Logs,
	}

	if task.Result != nil {
		response["result"] = task.Result
		if flowLog, ok := interface{}(task.Result).(interface {
			GetLogs() []oauth.LogEntry
		}); ok {
			if resultLogs := flowLog.GetLogs(); len(resultLogs) > 0 {
				response["logs"] = append(task.Logs, resultLogs...)
			}
		}
	}

	if task.Error != "" {
		response["error"] = task.Error
	}

	c.JSON(http.StatusOK, gin.H{
		"success": task.Status == "completed",
		"data":    response,
	})
}

func (s *Server) validateToken(c *gin.Context) {
	var req struct {
		ProviderID   string            `json:"providerId" binding:"required"`
		ProviderType string            `json:"providerType" binding:"required"`
		Credentials  map[string]string `json:"credentials" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	result, err := s.manager.ValidateToken(
		req.ProviderID,
		oauth.ProviderType(req.ProviderType),
		req.Credentials,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": result.Valid,
		"data":    result,
	})
}

func (s *Server) refreshToken(c *gin.Context) {
	var req struct {
		ProviderID   string            `json:"providerId" binding:"required"`
		ProviderType string            `json:"providerType" binding:"required"`
		Credentials  map[string]string `json:"credentials" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	result, err := s.manager.RefreshToken(
		req.ProviderID,
		oauth.ProviderType(req.ProviderType),
		req.Credentials,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": result != nil,
		"data":    result,
	})
}

func (s *Server) handleCallback(c *gin.Context) {
	code := c.Query("code")
	token := c.Query("token")
	state := c.Query("state")
	errorMsg := c.Query("error")

	result := gin.H{
		"success":      errorMsg == "",
		"code":         code,
		"token":        token,
		"state":        state,
		"error":        errorMsg,
		"description":  c.Query("error_description"),
		"callbackType": "oauth",
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) Start() error {
	s.setupRoutes()

	srv := &http.Server{
		Addr:    ":" + s.port,
		Handler: s.engine,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	log.Printf("API server started on http://localhost:%s\n", s.port)
	log.Println("Available endpoints:")
	log.Println("  GET  /api/v1/providers                              - List all providers")
	log.Println("  GET  /api/v1/providers/:type/accounts               - List accounts")
	log.Println("  POST /api/v1/providers/:type/accounts               - Create account")
	log.Println("  GET  /api/v1/providers/:type/accounts/:id           - Get account")
	log.Println("  PUT  /api/v1/providers/:type/accounts/:id           - Update account")
	log.Println("  DELETE /api/v1/providers/:type/accounts/:id         - Delete account")
	log.Println("  POST /api/v1/providers/:type/accounts/:id/validate  - Validate account")
	log.Println("  POST /api/v1/oauth/login/browser                    - Start browser automation")
	log.Println("  GET  /api/v1/oauth/browser/tasks/:taskId            - Get browser task status")
	log.Println("  POST /api/v1/oauth/validate                         - Validate token")
	log.Println("  POST /api/v1/oauth/refresh                          - Refresh token")
	log.Println("  GET  /health                                        - Health check")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
	return nil
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	server := NewServer(port)
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
