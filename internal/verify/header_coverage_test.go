package verify

import (
	"debug/elf"
	"debug/macho"
	"errors"
	"io"
	"os"
	"testing"
)

func TestMapGoArchToELF_AllCases(t *testing.T) {
	tests := []struct {
		goarch string
		want   elf.Machine
	}{
		{"amd64", elf.EM_X86_64},
		{"arm64", elf.EM_AARCH64},
		{"386", elf.EM_386},
		{"arm", elf.EM_ARM},
		{"mips", elf.EM_NONE},
		{"unknown", elf.EM_NONE},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			got := mapGoArchToELF(tt.goarch)
			if got != tt.want {
				t.Errorf("mapGoArchToELF(%q) = %v, want %v", tt.goarch, got, tt.want)
			}
		})
	}
}

func TestMapGoArchToMachO_AllCases(t *testing.T) {
	tests := []struct {
		goarch string
		want   macho.Cpu
	}{
		{"amd64", macho.CpuAmd64},
		{"arm64", macho.CpuArm64},
		{"386", macho.Cpu386},
		{"mips", macho.Cpu(0)},
		{"unknown", macho.Cpu(0)},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			got := mapGoArchToMachO(tt.goarch)
			if got != tt.want {
				t.Errorf("mapGoArchToMachO(%q) = %v, want %v", tt.goarch, got, tt.want)
			}
		})
	}
}

func TestMapELFMachine_AllCases(t *testing.T) {
	tests := []struct {
		machine elf.Machine
		want    string
	}{
		{elf.EM_X86_64, "x86_64"},
		{elf.EM_AARCH64, "arm64"},
		{elf.EM_386, "i386"},
		{elf.EM_ARM, "arm"},
		{elf.EM_MIPS, "unknown(8)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := mapELFMachine(tt.machine)
			if got != tt.want {
				t.Errorf("mapELFMachine(%v) = %q, want %q", tt.machine, got, tt.want)
			}
		})
	}
}

func TestMapMachOCpu_AllCases(t *testing.T) {
	tests := []struct {
		cpu  macho.Cpu
		want string
	}{
		{macho.CpuAmd64, "x86_64"},
		{macho.CpuArm64, "arm64"},
		{macho.Cpu386, "i386"},
		{macho.Cpu(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := mapMachOCpu(tt.cpu)
			if got != tt.want {
				t.Errorf("mapMachOCpu(%v) = %q, want %q", tt.cpu, got, tt.want)
			}
		})
	}
}

func TestElfTypeName(t *testing.T) {
	tests := []struct {
		typ  elf.Type
		want string
	}{
		{elf.ET_NONE, "unknown"},
		{elf.ET_REL, "relocatable object"},
		{elf.ET_EXEC, "executable"},
		{elf.ET_DYN, "shared object"},
		{elf.ET_CORE, "core dump"},
		{elf.Type(99), "unknown type (99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := elfTypeName(tt.typ)
			if got != tt.want {
				t.Errorf("elfTypeName(%v) = %q, want %q", tt.typ, got, tt.want)
			}
		})
	}
}

func TestMachoTypeName(t *testing.T) {
	tests := []struct {
		typ  macho.Type
		want string
	}{
		{macho.TypeObj, "object file"},
		{macho.TypeExec, "executable"},
		{macho.TypeDylib, "dynamic library"},
		{macho.TypeBundle, "bundle"},
		{macho.Type(99), "unknown type (99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := machoTypeName(tt.typ)
			if got != tt.want {
				t.Errorf("machoTypeName(%v) = %q, want %q", tt.typ, got, tt.want)
			}
		})
	}
}

func TestCategorizeELFError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantCat ErrorCategory
		wantMsg string
	}{
		{
			name:    "bad magic",
			err:     errors.New("bad magic number"),
			wantCat: ErrInvalidFormat,
			wantMsg: "invalid ELF magic number",
		},
		{
			name:    "EOF",
			err:     io.EOF,
			wantCat: ErrTruncated,
			wantMsg: "file is truncated",
		},
		{
			name:    "unexpected EOF",
			err:     io.ErrUnexpectedEOF,
			wantCat: ErrTruncated,
			wantMsg: "file is truncated",
		},
		{
			name:    "not exist",
			err:     os.ErrNotExist,
			wantCat: ErrUnreadable,
			wantMsg: "file not found",
		},
		{
			name:    "permission denied",
			err:     os.ErrPermission,
			wantCat: ErrUnreadable,
			wantMsg: "permission denied",
		},
		{
			name:    "offset error",
			err:     errors.New("invalid offset in section header"),
			wantCat: ErrCorrupted,
		},
		{
			name:    "section error",
			err:     errors.New("invalid section data"),
			wantCat: ErrCorrupted,
		},
		{
			name:    "generic error",
			err:     errors.New("something unexpected"),
			wantCat: ErrUnreadable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verr := categorizeELFError("/path/to/file", tt.err)
			if verr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", verr.Category, tt.wantCat)
			}
			if tt.wantMsg != "" && verr.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", verr.Message, tt.wantMsg)
			}
			if verr.Path != "/path/to/file" {
				t.Errorf("Path = %q, want %q", verr.Path, "/path/to/file")
			}
		})
	}
}

func TestCategorizeMachOError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantCat ErrorCategory
	}{
		{"invalid magic", errors.New("invalid magic number"), ErrInvalidFormat},
		{"EOF", io.EOF, ErrTruncated},
		{"not exist", os.ErrNotExist, ErrUnreadable},
		{"permission", os.ErrPermission, ErrUnreadable},
		{"command error", errors.New("invalid command in load"), ErrCorrupted},
		{"load error", errors.New("load section failed"), ErrCorrupted},
		{"generic error", errors.New("something else"), ErrUnreadable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verr := categorizeMachOError("/path/to/file", tt.err)
			if verr.Category != tt.wantCat {
				t.Errorf("Category = %v, want %v", verr.Category, tt.wantCat)
			}
		})
	}
}

func TestCategorizeFatError(t *testing.T) {
	t.Run("not fat", func(t *testing.T) {
		verr := categorizeFatError("/path/to/file", macho.ErrNotFat)
		if verr.Category != ErrInvalidFormat {
			t.Errorf("Category = %v, want %v", verr.Category, ErrInvalidFormat)
		}
	})

	t.Run("other error falls through to macho", func(t *testing.T) {
		verr := categorizeFatError("/path/to/file", os.ErrPermission)
		if verr.Category != ErrUnreadable {
			t.Errorf("Category = %v, want %v", verr.Category, ErrUnreadable)
		}
	})
}

func TestReadMagic_ShortFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/short.bin"
	if err := os.WriteFile(path, []byte{0x7f}, 0644); err != nil {
		t.Fatal(err)
	}

	magic, err := readMagic(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(magic) != 1 {
		t.Errorf("expected 1 byte, got %d", len(magic))
	}
}

func TestValidateHeader_PermissionDenied(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/noperm.so"
	if err := os.WriteFile(path, []byte{0x7f, 'E', 'L', 'F'}, 0000); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateHeader(path)
	if err == nil {
		// On some systems (root), this may succeed - skip
		t.Skip("permission test not applicable (possibly running as root)")
	}

	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if verr.Category != ErrUnreadable && verr.Category != ErrTruncated && verr.Category != ErrCorrupted {
		t.Errorf("Category = %v, want ErrUnreadable/ErrTruncated/ErrCorrupted", verr.Category)
	}
}

func TestErrorCategory_String_AllTier2(t *testing.T) {
	tests := []struct {
		cat    ErrorCategory
		expect string
	}{
		{ErrABIMismatch, "ABI mismatch"},
		{ErrUnknownDependency, "unknown dependency"},
		{ErrRpathLimitExceeded, "RPATH limit exceeded"},
		{ErrPathLengthExceeded, "path length exceeded"},
		{ErrUnexpandedVariable, "unexpanded path variable"},
		{ErrPathOutsideAllowed, "path outside allowed directories"},
		{ErrMaxDepthExceeded, "max depth exceeded"},
		{ErrMissingSoname, "missing soname"},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			got := tt.cat.String()
			if got != tt.expect {
				t.Errorf("String() = %q, want %q", got, tt.expect)
			}
		})
	}
}
