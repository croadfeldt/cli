package commands

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	catalogapi "github.com/dcm-project/control-plane/api/catalog/v1alpha1"
	"golang.org/x/crypto/ssh"
)

// defaultAuthorizedKeysPath is the UDLM Machine element that lists the SSH keys
// a machine's local accounts accept.
const defaultAuthorizedKeysPath = "authorized_keys"

var sshLoginRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// sshKeyRequest is what --generate-ssh-key and its companion flags ask for.
type sshKeyRequest struct {
	login    string // local account the key logs in to
	file     string // where the private key is written; the public key goes to file + ".pub"
	resource string // catalog item resource that takes the key; empty = the only one in the file
	path     string // user value path of the authorized keys list
}

// generateSSHKeyPair makes an ed25519 pair on this machine. The private half is
// returned in OpenSSH format and never leaves the caller; the public half is one
// authorized_keys line.
func generateSSHKeyPair(comment string) (privatePEM []byte, publicLine string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("generating key pair: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return nil, "", fmt.Errorf("encoding private key: %w", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, "", fmt.Errorf("encoding public key: %w", err)
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	if comment != "" {
		line += " " + comment
	}
	return pem.EncodeToMemory(block), line, nil
}

// writeSSHKeyFiles writes the pair and refuses to replace an existing file, so a
// key already in use is never lost.
func writeSSHKeyFiles(file string, privatePEM []byte, publicLine string) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists; choose another --ssh-key-file", file)
		}
		return fmt.Errorf("writing private key: %w", err)
	}
	if _, err := f.Write(privatePEM); err != nil {
		_ = f.Close()
		_ = os.Remove(file)
		return fmt.Errorf("writing private key: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(file)
		return fmt.Errorf("writing private key: %w", err)
	}
	if err := os.WriteFile(file+".pub", []byte(publicLine+"\n"), 0o644); err != nil { //nolint:gosec // a public key is public
		_ = os.Remove(file)
		return fmt.Errorf("writing public key: %w", err)
	}
	return nil
}

func removeSSHKeyFiles(file string) {
	_ = os.Remove(file)
	_ = os.Remove(file + ".pub")
}

// addAuthorizedKey adds {login, public_key} to the instance's authorized keys
// user value, creating that user value when the file does not have one.
func addAuthorizedKey(spec *catalogapi.CatalogItemInstanceSpec, req sshKeyRequest, publicLine string) error {
	resource := req.resource
	if resource == "" {
		seen := map[string]bool{}
		for _, uv := range spec.UserValues {
			seen[uv.Resource] = true
		}
		if len(seen) != 1 {
			return fmt.Errorf("the instance file names %d resources; say which takes the key with --ssh-key-resource", len(seen))
		}
		for r := range seen {
			resource = r
		}
	}
	entry := map[string]any{"login": req.login, "public_key": publicLine}
	for i := range spec.UserValues {
		uv := &spec.UserValues[i]
		if uv.Resource != resource || uv.Path != req.path {
			continue
		}
		list, ok := uv.Value.([]any)
		if !ok {
			return fmt.Errorf("user value %s.%s is not a list", resource, req.path)
		}
		uv.Value = append(list, entry)
		return nil
	}
	spec.UserValues = append(spec.UserValues, catalogapi.UserValue{
		Resource: resource, Path: req.path, Value: []any{entry},
	})
	return nil
}

// prepareSSHKey generates the pair, writes it, and adds the public half to the
// request. It returns a cleanup that removes the files if the request then fails.
func prepareSSHKey(spec *catalogapi.CatalogItemInstanceSpec, req sshKeyRequest, comment string) (cleanup func(), err error) {
	if !sshLoginRe.MatchString(req.login) {
		return nil, fmt.Errorf("--generate-ssh-key %q is not a local account name", req.login)
	}
	if req.file == "" {
		req.file = req.login + "_ed25519"
	}
	privatePEM, publicLine, err := generateSSHKeyPair(comment)
	if err != nil {
		return nil, err
	}
	if err := addAuthorizedKey(spec, req, publicLine); err != nil {
		return nil, err
	}
	if err := writeSSHKeyFiles(req.file, privatePEM, publicLine); err != nil {
		return nil, err
	}
	return func() { removeSSHKeyFiles(req.file) }, nil
}
