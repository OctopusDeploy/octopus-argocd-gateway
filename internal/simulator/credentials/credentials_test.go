package credentials

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "creds.json")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestLoad_AppliesDefaults(t *testing.T) {
	p := writeTemp(t, `{
	  "default_server_url": "octopus:443",
	  "default_thumbprint": "ABCD",
	  "gateways": [
	    {"client_id": "g-1", "bearer_token": "t1"},
	    {"client_id": "g-2", "bearer_token": "t2", "server_url": "other:443"}
	  ]
	}`)
	f, err := Load(p)
	require.NoError(t, err)
	require.Len(t, f.Gateways, 2)

	assert.Equal(t, "octopus:443", f.Gateways[0].ServerURL)
	assert.Equal(t, "ABCD", f.Gateways[0].Thumbprint)
	assert.Equal(t, "other:443", f.Gateways[1].ServerURL, "per-entry override wins")
	assert.Equal(t, "ABCD", f.Gateways[1].Thumbprint)
}

func TestLoad_RejectsMissingClientID(t *testing.T) {
	p := writeTemp(t, `{"default_server_url":"x:1","gateways":[{"bearer_token":"t"}]}`)
	_, err := Load(p)
	assert.ErrorContains(t, err, "client_id")
}

func TestLoad_RejectsMissingBearerToken(t *testing.T) {
	p := writeTemp(t, `{"default_server_url":"x:1","gateways":[{"client_id":"a"}]}`)
	_, err := Load(p)
	assert.ErrorContains(t, err, "bearer_token")
}

func TestLoad_RejectsNoServerURL(t *testing.T) {
	p := writeTemp(t, `{"gateways":[{"client_id":"a","bearer_token":"t"}]}`)
	_, err := Load(p)
	assert.ErrorContains(t, err, "server_url")
}

func TestLoad_RejectsEmptyGateways(t *testing.T) {
	p := writeTemp(t, `{"default_server_url":"x:1","gateways":[]}`)
	_, err := Load(p)
	assert.ErrorContains(t, err, "no gateways")
}

func TestSelect_CapsAndReturnsAll(t *testing.T) {
	f := &File{
		Gateways: []Entry{
			{ClientID: "a"}, {ClientID: "b"}, {ClientID: "c"},
		},
	}
	assert.Len(t, f.Select(0), 3)
	assert.Len(t, f.Select(10), 3)
	assert.Len(t, f.Select(2), 2)
	assert.Equal(t, "a", f.Select(2)[0].ClientID)
	assert.Equal(t, "b", f.Select(2)[1].ClientID)
}
