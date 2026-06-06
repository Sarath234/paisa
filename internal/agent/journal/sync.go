package journal

import (
	"fmt"
	"net/http"
)

func TriggerSync(paisaURL, apiToken string) error {
	req, err := http.NewRequest("POST", paisaURL+"/api/sync", nil)
	if err != nil {
		return err
	}
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sync request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("sync returned %d", resp.StatusCode)
	}
	return nil
}
