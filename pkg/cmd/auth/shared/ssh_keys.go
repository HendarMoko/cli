package shared

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/ssh-key/add"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/safeexec"
)

type SshContext struct {
	configDir string
	keygenExe string
}

func (c *SshContext) sshDir() (string, error) {
	if c.configDir != "" {
		return c.configDir, nil
	}
	dir, err := config.HomeDirPath(".ssh")
	if err == nil {
		c.configDir = dir
	}
	return dir, err
}

func (c *SshContext) localPublicKeys() ([]string, error) {
	sshDir, err := c.sshDir()
	if err != nil {
		return nil, err
	}

	return filepath.Glob(filepath.Join(sshDir, "*.pub"))
}

func (c *SshContext) findKeygen() (string, error) {
	if c.keygenExe != "" {
		return c.keygenExe, nil
	}

	keygenExe, err := safeexec.LookPath("ssh-keygen")
	if err != nil && runtime.GOOS == "windows" {
		// We can try and find ssh-keygen in a Git for Windows install
		if gitPath, err := safeexec.LookPath("git"); err == nil {
			gitKeygen := filepath.Join(filepath.Dir(gitPath), "..", "usr", "bin", "ssh-keygen.exe")
			if _, err = os.Stat(gitKeygen); err == nil {
				return gitKeygen, nil
			}
		}
	}

	if err == nil {
		c.keygenExe = keygenExe
	}
	return keygenExe, err
}

func (c *SshContext) GenerateSSHKey() (string, error) {
	return c.GenerateSSHKeyWithOptions("id_ed25519", true)
}

func (c *SshContext) GenerateSSHKeyWithOptions(keyName string, errorOnExists bool) (string, error) {
	keygenExe, err := c.findKeygen()
	if err != nil {
		// give up silently if `ssh-keygen` is not available
		return "", nil
	}

	// TODO: Prompt after searching for existing key
	var sshChoice bool
	err = prompt.SurveyAskOne(&survey.Confirm{
		// TODO: Change this message if we're not uploading
		Message: "Generate a new SSH key to add to your GitHub account?",
		Default: true,
	}, &sshChoice)
	if err != nil {
		return "", fmt.Errorf("could not prompt: %w", err)
	}
	if !sshChoice {
		return "", nil
	}

	sshDir, err := c.sshDir()
	if err != nil {
		return "", err
	}
	keyFile := filepath.Join(sshDir, keyName)
	if _, err := os.Stat(keyFile); err == nil {
		if errorOnExists {
			return "", fmt.Errorf("refusing to overwrite file %s", keyFile)
		} else {
			return keyFile + ".pub", nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(keyFile), 0711); err != nil {
		return "", err
	}

	var sshLabel string
	var sshPassphrase string
	err = prompt.SurveyAskOne(&survey.Password{
		Message: "Enter a passphrase for your new SSH key (Optional)",
	}, &sshPassphrase)
	if err != nil {
		return "", fmt.Errorf("could not prompt: %w", err)
	}

	keygenCmd := exec.Command(keygenExe, "-t", "ed25519", "-C", sshLabel, "-N", sshPassphrase, "-f", keyFile)
	return keyFile + ".pub", run.PrepareCmd(keygenCmd).Run()
}

func sshKeyUpload(httpClient *http.Client, hostname, keyFile string, title string) error {
	f, err := os.Open(keyFile)
	if err != nil {
		return err
	}
	defer f.Close()

	return add.SSHKeyUpload(httpClient, hostname, f, title)
}
