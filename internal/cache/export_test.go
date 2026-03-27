package cache

import "os"

// EvictEntries exposes evictEntries for testing.
var EvictEntries = evictEntries

// FakeDirEntry is a synthetic os.DirEntry for testing eviction edge cases.
type FakeDirEntry struct {
	FakeName  string
	FakeIsDir bool
	FakeInfo  os.FileInfo
	FakeErr   error
}

func (f FakeDirEntry) Name() string               { return f.FakeName }
func (f FakeDirEntry) IsDir() bool                { return f.FakeIsDir }
func (f FakeDirEntry) Type() os.FileMode          { return 0 }
func (f FakeDirEntry) Info() (os.FileInfo, error)  { return f.FakeInfo, f.FakeErr }
