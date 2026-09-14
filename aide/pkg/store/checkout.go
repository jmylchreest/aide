package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jmylchreest/aide/aide/pkg/checkout"
)

// CheckoutRoot preserves the caller's working copy while memory stays shared.
func CheckoutRoot(dbPath string) string {
	cwd, _ := os.Getwd()
	return checkout.RootFor(ProjectRootFromDB(dbPath), cwd)
}

func CheckoutInfo(dbPath, root string) (checkout.Info, error) {
	return checkout.Resolve(ProjectRootFromDB(dbPath), root)
}

func CheckoutDir(dbPath string, c checkout.Info) string {
	return filepath.Join(filepath.Dir(dbPath), "checkouts", c.ID)
}

// RegisterCheckout is called by the shared database owner, before opening any
// generated stores. Keeping metadata outside those stores permits safe pruning.
func RegisterCheckout(dbPath string, c checkout.Info) error {
	dir := CheckoutDir(dbPath, c)
	if err := checkout.EnsureIgnoredDir(filepath.Dir(dir)); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".checkout-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	ce := tmp.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "checkout.json"))
}
