package commands_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"

	"github.com/dcm-project/cli/internal/commands"
)

var _ = Describe("Catalog instance create with a generated SSH key", func() {
	var (
		server  *httptest.Server
		errBuf  *bytes.Buffer
		sent    map[string]any
		status  int
		keyFile string
	)

	const oneResource = "display_name: bastion-01\napi_version: v1alpha1\nspec:\n  catalog_item_id: small-vm\n  user_values:\n    - resource: vm\n      path: instance_size\n      value: small\n"

	BeforeEach(func() {
		clearDCMEnvVars()
		sent = nil
		status = http.StatusCreated
		keyFile = filepath.Join(GinkgoT().TempDir(), "id_bastion")
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(json.NewDecoder(r.Body).Decode(&sent)).To(Succeed())
			if status != http.StatusCreated {
				writeRFC7807(w, status, "about:blank", "Bad Request", "refused")
				return
			}
			writeJSONResponse(w, status, sampleInstanceResponse())
		}))
	})

	AfterEach(func() { server.Close() })

	run := func(args ...string) error {
		cmd := commands.NewRootCommand()
		errBuf = new(bytes.Buffer)
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetErr(errBuf)
		cmd.SetArgs(append([]string{"--config", nonexistentConfigPath(), "--control-plane-url", server.URL,
			"catalog", "instance", "create"}, args...))
		return cmd.Execute()
	}

	userValue := func(resource, path string) any {
		spec, _ := sent["spec"].(map[string]any)
		values, _ := spec["user_values"].([]any)
		for _, v := range values {
			m, _ := v.(map[string]any)
			if m["resource"] == resource && m["path"] == path {
				return m["value"]
			}
		}
		return nil
	}

	It("sends only the public key and keeps the private key on this machine", func() {
		Expect(run("--from-file", writeTempFile(oneResource, ".yaml"),
			"--generate-ssh-key", "cloud-user", "--ssh-key-file", keyFile)).To(Succeed())

		keys, ok := userValue("vm", "authorized_keys").([]any)
		Expect(ok).To(BeTrue(), "the request carries an authorized_keys user value")
		Expect(keys).To(HaveLen(1))
		entry, _ := keys[0].(map[string]any)
		Expect(entry["login"]).To(Equal("cloud-user"))
		sentKey, _ := entry["public_key"].(string)
		Expect(sentKey).To(HavePrefix("ssh-ed25519 "))
		Expect(sentKey).To(HaveSuffix(" cloud-user@bastion-01"))

		raw, _ := json.Marshal(sent)
		Expect(string(raw)).NotTo(ContainSubstring("PRIVATE KEY"))

		info, err := os.Stat(keyFile)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
		privatePEM, err := os.ReadFile(keyFile)
		Expect(err).NotTo(HaveOccurred())
		signer, err := ssh.ParsePrivateKey(privatePEM)
		Expect(err).NotTo(HaveOccurred())
		derived := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
		Expect(sentKey).To(HavePrefix(derived), "the key sent is the public half of the key written")

		pubFile, err := os.ReadFile(keyFile + ".pub")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(string(pubFile))).To(Equal(sentKey))
		Expect(errBuf.String()).To(ContainSubstring(keyFile))
	})

	It("adds to an authorized_keys list the file already has", func() {
		file := oneResource + "    - resource: vm\n      path: authorized_keys\n      value:\n        - login: ops\n          credential_ref: credential/ops-ssh-key\n"
		Expect(run("--from-file", writeTempFile(file, ".yaml"),
			"--generate-ssh-key", "cloud-user", "--ssh-key-file", keyFile)).To(Succeed())
		keys, _ := userValue("vm", "authorized_keys").([]any)
		Expect(keys).To(HaveLen(2))
		first, _ := keys[0].(map[string]any)
		Expect(first["credential_ref"]).To(Equal("credential/ops-ssh-key"))
	})

	It("needs --ssh-key-resource when the file names several resources", func() {
		file := oneResource + "    - resource: db\n      path: instance_size\n      value: small\n"
		err := run("--from-file", writeTempFile(file, ".yaml"),
			"--generate-ssh-key", "cloud-user", "--ssh-key-file", keyFile)
		Expect(err).To(MatchError(ContainSubstring("--ssh-key-resource")))
		Expect(sent).To(BeNil(), "nothing is sent")
		Expect(keyFile).NotTo(BeAnExistingFile())

		Expect(run("--from-file", writeTempFile(file, ".yaml"), "--generate-ssh-key", "cloud-user",
			"--ssh-key-file", keyFile, "--ssh-key-resource", "db")).To(Succeed())
		Expect(userValue("db", "authorized_keys")).NotTo(BeNil())
		Expect(userValue("vm", "authorized_keys")).To(BeNil())
	})

	It("never replaces an existing key file", func() {
		Expect(os.WriteFile(keyFile, []byte("in use"), 0o600)).To(Succeed())
		err := run("--from-file", writeTempFile(oneResource, ".yaml"),
			"--generate-ssh-key", "cloud-user", "--ssh-key-file", keyFile)
		Expect(err).To(MatchError(ContainSubstring("already exists")))
		Expect(sent).To(BeNil(), "nothing is sent")
		kept, _ := os.ReadFile(keyFile)
		Expect(string(kept)).To(Equal("in use"))
	})

	It("removes the key files when the control plane refuses the request", func() {
		status = http.StatusBadRequest
		Expect(run("--from-file", writeTempFile(oneResource, ".yaml"),
			"--generate-ssh-key", "cloud-user", "--ssh-key-file", keyFile)).NotTo(Succeed())
		Expect(keyFile).NotTo(BeAnExistingFile())
		Expect(keyFile + ".pub").NotTo(BeAnExistingFile())
	})

	It("refuses a login that is not a local account name", func() {
		err := run("--from-file", writeTempFile(oneResource, ".yaml"),
			"--generate-ssh-key", "Bad User", "--ssh-key-file", keyFile)
		Expect(err).To(MatchError(ContainSubstring("not a local account name")))
		Expect(sent).To(BeNil())
	})

	It("sends nothing extra without the flag", func() {
		Expect(run("--from-file", writeTempFile(oneResource, ".yaml"))).To(Succeed())
		Expect(userValue("vm", "authorized_keys")).To(BeNil())
	})
})
