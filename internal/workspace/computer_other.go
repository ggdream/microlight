//go:build !windows && !darwin

package workspace

func NewLocalComputer() ComputerDriver { return nil }
