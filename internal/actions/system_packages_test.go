package actions

import "testing"

func TestAptInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}
	if action.Name() != "apt_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "apt_install")
	}
}

func TestAptInstallAction_Execute(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"build-essential", "libssl-dev"},
	})
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

func TestAptInstallAction_Execute_MissingPackages(t *testing.T) {
	t.Parallel()
	action := &AptInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'packages' parameter is missing")
	}
}

func TestYumInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &YumInstallAction{}
	if action.Name() != "yum_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "yum_install")
	}
}

func TestYumInstallAction_Execute(t *testing.T) {
	t.Parallel()
	action := &YumInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"gcc", "openssl-devel"},
	})
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

func TestYumInstallAction_Execute_MissingPackages(t *testing.T) {
	t.Parallel()
	action := &YumInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'packages' parameter is missing")
	}
}

func TestBrewInstallAction_Name(t *testing.T) {
	t.Parallel()
	action := &BrewInstallAction{}
	if action.Name() != "brew_install" {
		t.Errorf("Name() = %q, want %q", action.Name(), "brew_install")
	}
}

func TestBrewInstallAction_Execute(t *testing.T) {
	t.Parallel()
	action := &BrewInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{
		"packages": []interface{}{"openssl", "libyaml"},
	})
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}
}

func TestBrewInstallAction_Execute_MissingPackages(t *testing.T) {
	t.Parallel()
	action := &BrewInstallAction{}
	tmpDir := t.TempDir()

	ctx := &ExecutionContext{
		WorkDir:    tmpDir,
		InstallDir: tmpDir,
		Version:    "1.0.0",
	}

	err := action.Execute(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("Execute() should fail when 'packages' parameter is missing")
	}
}
