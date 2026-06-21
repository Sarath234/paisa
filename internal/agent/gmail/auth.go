package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googlemail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// NewService creates an authenticated Gmail service. On first run, opens
// a browser for OAuth2 consent and saves the token to tokenFile.
func NewService(ctx context.Context, credentialsFile, tokenFile string) (*googlemail.Service, error) {
	creds, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	oauthConfig, err := google.ConfigFromJSON(creds, googlemail.GmailReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	tok, err := loadToken(tokenFile)
	if err != nil {
		tok, err = getTokenFromWeb(oauthConfig, tokenFile)
		if err != nil {
			return nil, err
		}
	}

	client := oauthConfig.Client(ctx, tok)
	return googlemail.NewService(ctx, option.WithHTTPClient(client))
}

func loadToken(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	return tok, json.NewDecoder(f).Decode(tok)
}

func getTokenFromWeb(config *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	config.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Open this URL in your browser:\n%v\n\nEnter authorization code: ", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("read auth code: %w", err)
	}

	tok, err := config.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("exchange token: %w", err)
	}

	f, err := os.OpenFile(tokenFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return tok, json.NewEncoder(f).Encode(tok)
}

