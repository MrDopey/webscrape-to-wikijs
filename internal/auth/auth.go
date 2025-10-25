package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// DriveService wraps the Google Drive API service
type DriveService struct {
	Service *drive.Service
	ctx     context.Context
}

// NewDriveService creates a new Drive service from credentials file
func NewDriveService(ctx context.Context, credentialsPath string) (*DriveService, error) {
	// Read credentials file
	credBytes, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	// Try service account first
	// Use DriveScope to allow reading existing files, metadata, and creating temporary files for PDF conversion
	config, err := google.JWTConfigFromJSON(credBytes, drive.DriveScope)
	if err == nil {
		// Service account authentication
		client := config.Client(ctx)
		srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			return nil, fmt.Errorf("failed to create Drive service: %w", err)
		}
		return &DriveService{Service: srv, ctx: ctx}, nil
	}

	// Try OAuth2 credentials
	var creds struct {
		Web struct {
			ClientID     string   `json:"client_id"`
			ClientSecret string   `json:"client_secret"`
			RedirectURIs []string `json:"redirect_uris"`
		} `json:"web"`
		Installed struct {
			ClientID     string   `json:"client_id"`
			ClientSecret string   `json:"client_secret"`
			RedirectURIs []string `json:"redirect_uris"`
		} `json:"installed"`
	}

	if err := json.Unmarshal(credBytes, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials file: %w", err)
	}

	// Use installed app credentials if available, otherwise web
	var oauthConfig *oauth2.Config
	if creds.Installed.ClientID != "" {
		oauthConfig = &oauth2.Config{
			ClientID:     creds.Installed.ClientID,
			ClientSecret: creds.Installed.ClientSecret,
			RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
			Scopes:       []string{drive.DriveScope}, // Full Drive access: read existing files + create temp files for PDF conversion
			Endpoint:     google.Endpoint,
		}
	} else if creds.Web.ClientID != "" {
		oauthConfig = &oauth2.Config{
			ClientID:     creds.Web.ClientID,
			ClientSecret: creds.Web.ClientSecret,
			RedirectURL:  creds.Web.RedirectURIs[0],
			Scopes:       []string{drive.DriveScope}, // Full Drive access: read existing files + create temp files for PDF conversion
			Endpoint:     google.Endpoint,
		}
	} else {
		return nil, fmt.Errorf("invalid credentials format: expected service account or OAuth2 credentials")
	}

	// Try to load saved token
	token, err := loadToken()
	if err != nil || token == nil {
		// No saved token, get new token from user
		token, err = getTokenFromWeb(ctx, oauthConfig)
		if err != nil {
			return nil, err
		}
		// Save token for future use
		if err := saveToken(token); err != nil {
			fmt.Printf("Warning: Failed to save token: %v\n", err)
		}
	}

	client := oauthConfig.Client(ctx, token)
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create Drive service: %w", err)
	}

	return &DriveService{Service: srv, ctx: ctx}, nil
}

// Context returns the context associated with this service
func (ds *DriveService) Context() context.Context {
	return ds.ctx
}

// getTokenFromWeb uses OAuth2 to retrieve a token from the web
func getTokenFromWeb(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser:\n%v\n", authURL)
	fmt.Print("Enter authorization code: ")

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("failed to read authorization code: %w", err)
	}

	token, err := config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	return token, nil
}

// getTokenPath returns the path to the token file
func getTokenPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	tokenDir := filepath.Join(homeDir, ".credentials")
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create credentials directory: %w", err)
	}

	return filepath.Join(tokenDir, "gdrive-crawler-token.json"), nil
}

// saveToken saves a token to a file
func saveToken(token *oauth2.Token) error {
	tokenPath, err := getTokenPath()
	if err != nil {
		return err
	}

	fmt.Printf("Saving credentials to: %s\n", tokenPath)
	f, err := os.OpenFile(tokenPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create token file: %w", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(token); err != nil {
		return fmt.Errorf("failed to encode token: %w", err)
	}

	return nil
}

// loadToken loads a token from a file
func loadToken() (*oauth2.Token, error) {
	tokenPath, err := getTokenPath()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No token file exists
		}
		return nil, fmt.Errorf("failed to open token file: %w", err)
	}
	defer f.Close()

	token := &oauth2.Token{}
	if err := json.NewDecoder(f).Decode(token); err != nil {
		return nil, fmt.Errorf("failed to decode token: %w", err)
	}

	return token, nil
}
