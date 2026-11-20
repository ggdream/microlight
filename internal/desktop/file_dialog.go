//go:build !darwin || ios

package desktop

import "github.com/egoist/mygo"

// OpenFilteredFiles uses MyGo's native picker. Desktop hosts select the first
// filter; mobile document pickers retain their system-controlled filtering UI.
func OpenFilteredFiles(options mygo.OpenDialogOptions) ([]string, error) {
	return mygo.Dialog.Open(options)
}
