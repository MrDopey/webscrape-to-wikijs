package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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
	config, err := google.JWTConfigFromJSON(credBytes, drive.DriveReadonlyScope)
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
			Scopes:       []string{drive.DriveReadonlyScope},
			Endpoint:     google.Endpoint,
		}
	} else if creds.Web.ClientID != "" {
		oauthConfig = &oauth2.Config{
			ClientID:     creds.Web.ClientID,
			ClientSecret: creds.Web.ClientSecret,
			RedirectURL:  creds.Web.RedirectURIs[0],
			Scopes:       []string{drive.DriveReadonlyScope},
			Endpoint:     google.Endpoint,
		}
	} else {
		return nil, fmt.Errorf("invalid credentials format: expected service account or OAuth2 credentials")
	}

	// Get token from user
	authURL := oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser:\n%v\n", authURL)
	fmt.Print("Enter authorization code: ")

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("failed to read authorization code: %w", err)
	}

	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
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
