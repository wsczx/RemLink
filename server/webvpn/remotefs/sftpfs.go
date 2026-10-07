package remotefs

import (
	"io"
	"io/fs"
	"os"

	"github.com/pkg/sftp"
)

// 基于 github.com/pkg/sftp 的 RemoteFS 实现
type SFTPFS struct {
	C *sftp.Client
}

func NewSFTP(c *sftp.Client) *SFTPFS { return &SFTPFS{C: c} }

func (s *SFTPFS) List(p string) ([]fs.FileInfo, error) { return s.C.ReadDir(p) }
func (s *SFTPFS) Stat(p string) (fs.FileInfo, error)   { return s.C.Stat(p) }
func (s *SFTPFS) Lstat(p string) (fs.FileInfo, error)  { return s.C.Lstat(p) }
func (s *SFTPFS) ReadLink(p string) (string, error)    { return s.C.ReadLink(p) }

func (s *SFTPFS) Open(p string) (File, error) {
	f, err := s.C.Open(p)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *SFTPFS) Create(p string, offset int64) (io.WriteCloser, error) {
	if offset <= 0 {
		return s.C.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	}
	f, err := s.C.OpenFile(p, os.O_WRONLY)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (s *SFTPFS) Mkdir(p string) error    { return s.C.Mkdir(p) }
func (s *SFTPFS) MkdirAll(p string) error { return MkdirAllGeneric(s, p) }

func (s *SFTPFS) Remove(p string) error {
	fi, err := s.C.Lstat(p)
	if err != nil {
		return err
	}
	if fi.IsDir() && fi.Mode()&fs.ModeSymlink == 0 {
		return s.C.RemoveDirectory(p)
	}
	return s.C.Remove(p)
}

func (s *SFTPFS) RemoveAll(p string) error     { return RemoveAllGeneric(s, p) }
func (s *SFTPFS) Rename(from, to string) error { return s.C.Rename(from, to) }
func (s *SFTPFS) Close() error                 { return s.C.Close() }
