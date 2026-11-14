//go:build !windows

package workspace

func NewLocalComputer() ComputerDriver { return nil }
