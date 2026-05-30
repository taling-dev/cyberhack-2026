package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// KeycloakAdmin syncs realm-role assignments to Keycloak so that role changes
// made via AdminService are reflected in users' JWTs on their next token
// refresh. It authenticates with the client_credentials grant of a confidential
// service-account client that holds the realm-management `manage-users` role.
//
// Configuration (env):
//   KEYCLOAK_INTERNAL_URL   — e.g. http://keycloak.platform:8080/realms/simaops
//   KEYCLOAK_ADMIN_CLIENT_ID     — service-account client id (e.g. simaops-api)
//   KEYCLOAK_ADMIN_CLIENT_SECRET — its secret
//
// If the client id/secret are absent the admin client is disabled and all
// methods are graceful no-ops (returning nil), so deployments without the
// service account behave exactly as before — local DB writes still happen,
// only the Keycloak mirror is skipped.
type KeycloakAdmin struct {
	enabled      bool
	serverURL    string // scheme://host:port  (no /realms/...)
	realm        string
	clientID     string
	clientSecret string
	hc           *http.Client
}

func NewKeycloakAdmin() *KeycloakAdmin {
	internal := os.Getenv("KEYCLOAK_INTERNAL_URL")
	if internal == "" {
		internal = os.Getenv("KEYCLOAK_ISSUER")
	}
	clientID := os.Getenv("KEYCLOAK_ADMIN_CLIENT_ID")
	secret := os.Getenv("KEYCLOAK_ADMIN_CLIENT_SECRET")

	// Derive server root + realm from the issuer/internal URL of the form
	// scheme://host[:port]/realms/<realm>.
	server, realm := "", ""
	if i := strings.Index(internal, "/realms/"); i >= 0 {
		server = internal[:i]
		realm = strings.TrimSuffix(internal[i+len("/realms/"):], "/")
	}

	return &KeycloakAdmin{
		enabled:      clientID != "" && secret != "" && server != "" && realm != "",
		serverURL:    server,
		realm:        realm,
		clientID:     clientID,
		clientSecret: secret,
		hc:           &http.Client{Timeout: 10 * time.Second},
	}
}

func (k *KeycloakAdmin) Enabled() bool { return k != nil && k.enabled }

func (k *KeycloakAdmin) token(ctx context.Context) (string, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {k.clientID},
		"client_secret": {k.clientSecret},
	}
	endpoint := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", k.serverURL, k.realm)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := k.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak token: status %d", res.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.AccessToken, nil
}

type kcRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// getUserIDByUsername resolves a Keycloak user UUID from a username. We look up
// by username rather than trusting a caller-supplied id because the local
// users_profile.id values are synthetic seed placeholders (e.g. "u-admin"),
// not the Keycloak `sub` UUID the Admin API requires.
func (k *KeycloakAdmin) getUserIDByUsername(ctx context.Context, token, username string) (string, error) {
	endpoint := fmt.Sprintf("%s/admin/realms/%s/users?username=%s&exact=true",
		k.serverURL, k.realm, url.QueryEscape(username))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := k.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lookup user %q: status %d", username, res.StatusCode)
	}
	var users []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&users); err != nil {
		return "", err
	}
	if len(users) == 0 {
		return "", fmt.Errorf("keycloak user %q not found", username)
	}
	return users[0].ID, nil
}

func (k *KeycloakAdmin) getRealmRole(ctx context.Context, token, roleName string) (kcRole, error) {
	var r kcRole
	endpoint := fmt.Sprintf("%s/admin/realms/%s/roles/%s", k.serverURL, k.realm, url.PathEscape(roleName))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := k.hc.Do(req)
	if err != nil {
		return r, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return r, fmt.Errorf("get realm role %q: status %d", roleName, res.StatusCode)
	}
	return r, json.NewDecoder(res.Body).Decode(&r)
}

// roleMapping POSTs (assign) or DELETEs (revoke) a realm role on a user,
// resolving the Keycloak user UUID from the given username first.
func (k *KeycloakAdmin) roleMapping(ctx context.Context, method, username, roleName string) error {
	token, err := k.token(ctx)
	if err != nil {
		return err
	}
	userID, err := k.getUserIDByUsername(ctx, token, username)
	if err != nil {
		return err
	}
	role, err := k.getRealmRole(ctx, token, roleName)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal([]kcRole{role})
	endpoint := fmt.Sprintf("%s/admin/realms/%s/users/%s/role-mappings/realm", k.serverURL, k.realm, url.PathEscape(userID))
	req, _ := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := k.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s role-mapping %q on %q: status %d", method, roleName, username, res.StatusCode)
	}
	return nil
}

// AssignRealmRole mirrors a role assignment to Keycloak. No-op if disabled.
func (k *KeycloakAdmin) AssignRealmRole(ctx context.Context, username, roleName string) error {
	if !k.Enabled() {
		return nil
	}
	return k.roleMapping(ctx, http.MethodPost, username, roleName)
}

// RemoveRealmRole mirrors a role revocation to Keycloak. No-op if disabled.
func (k *KeycloakAdmin) RemoveRealmRole(ctx context.Context, username, roleName string) error {
	if !k.Enabled() {
		return nil
	}
	return k.roleMapping(ctx, http.MethodDelete, username, roleName)
}
