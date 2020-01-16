// Copyright 2019 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package linux

import (
	"strings"

	"gvisor.dev/gvisor/pkg/abi/linux"
	"gvisor.dev/gvisor/pkg/sentry/arch"
	"gvisor.dev/gvisor/pkg/sentry/fs"
	"gvisor.dev/gvisor/pkg/sentry/kernel"
	"gvisor.dev/gvisor/pkg/sentry/usermem"
	"gvisor.dev/gvisor/pkg/syserror"
)

// GetXattr implements linux syscall getxattr(2).
func GetXattr(t *kernel.Task, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	pathAddr := args[0].Pointer()
	nameAddr := args[1].Pointer()
	valueAddr := args[2].Pointer()
	size := uint64(args[3].SizeT())

	path, dirPath, err := copyInPath(t, pathAddr, false /* allowEmpty */)
	if err != nil {
		return 0, nil, err
	}

	valueLen := 0
	err = fileOpOn(t, linux.AT_FDCWD, path, true /* resolve */, func(root *fs.Dirent, d *fs.Dirent, _ uint) error {
		// If getxattr(2) is called with size 0, the size of the value will be
		// returned successfully even if it is nonzero. In that case, we need to
		// retrieve the entire attribute value so we can return the correct size.
		requestedSize := size
		if size == 0 || size > linux.XATTR_SIZE_MAX {
			requestedSize = linux.XATTR_SIZE_MAX
		}

		value, err := getXattr(t, d, dirPath, nameAddr, uint64(requestedSize))
		if err != nil {
			return err
		}

		valueLen = len(value)
		if uint64(valueLen) > requestedSize {
			return syserror.ERANGE
		}

		// Skip copying out the attribute value if size is 0.
		if size == 0 {
			return nil
		}
		_, err = t.CopyOutBytes(valueAddr, []byte(value))
		return err
	})
	if err != nil {
		return 0, nil, err
	}
	return uintptr(valueLen), nil, nil
}

// getXattr implements getxattr(2) from the given *fs.Dirent.
func getXattr(t *kernel.Task, d *fs.Dirent, dirPath bool, nameAddr usermem.Addr, size uint64) (string, error) {
	if dirPath && !fs.IsDir(d.Inode.StableAttr) {
		return "", syserror.ENOTDIR
	}

	if err := checkXattrPermissions(t, d.Inode, fs.PermMask{Read: true}); err != nil {
		return "", err
	}

	name, err := copyInXattrName(t, nameAddr)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(name, linux.XATTR_USER_PREFIX) {
		return "", syserror.EOPNOTSUPP
	}

	return d.Inode.GetXattr(t, name, size)
}

// SetXattr implements linux syscall setxattr(2).
func SetXattr(t *kernel.Task, args arch.SyscallArguments) (uintptr, *kernel.SyscallControl, error) {
	pathAddr := args[0].Pointer()
	nameAddr := args[1].Pointer()
	valueAddr := args[2].Pointer()
	size := uint64(args[3].SizeT())
	flags := args[4].Uint()

	path, dirPath, err := copyInPath(t, pathAddr, false /* allowEmpty */)
	if err != nil {
		return 0, nil, err
	}

	if flags&^(linux.XATTR_CREATE|linux.XATTR_REPLACE) != 0 {
		return 0, nil, syserror.EINVAL
	}

	return 0, nil, fileOpOn(t, linux.AT_FDCWD, path, true /* resolve */, func(root *fs.Dirent, d *fs.Dirent, _ uint) error {
		return setXattr(t, d, dirPath, nameAddr, valueAddr, uint64(size), flags)
	})
}

// setXattr implements setxattr(2) from the given *fs.Dirent.
func setXattr(t *kernel.Task, d *fs.Dirent, dirPath bool, nameAddr, valueAddr usermem.Addr, size uint64, flags uint32) error {
	if dirPath && !fs.IsDir(d.Inode.StableAttr) {
		return syserror.ENOTDIR
	}

	if err := checkXattrPermissions(t, d.Inode, fs.PermMask{Write: true}); err != nil {
		return err
	}

	name, err := copyInXattrName(t, nameAddr)
	if err != nil {
		return err
	}

	if size > linux.XATTR_SIZE_MAX {
		return syserror.E2BIG
	}
	buf := make([]byte, size)
	if _, err = t.CopyInBytes(valueAddr, buf); err != nil {
		return err
	}
	value := string(buf)

	if !strings.HasPrefix(name, linux.XATTR_USER_PREFIX) {
		return syserror.EOPNOTSUPP
	}

	return d.Inode.SetXattr(t, name, value, flags)
}

func copyInXattrName(t *kernel.Task, nameAddr usermem.Addr) (string, error) {
	name, err := t.CopyInString(nameAddr, linux.XATTR_NAME_MAX+1)
	if err != nil {
		if err == syserror.ENAMETOOLONG {
			return "", syserror.ERANGE
		}
		return "", err
	}
	if len(name) == 0 {
		return "", syserror.ERANGE
	}
	return name, nil
}

func checkXattrPermissions(t *kernel.Task, i *fs.Inode, perms fs.PermMask) error {
	// Restrict xattrs to regular files and directories.
	//
	// In Linux, this restriction technically only applies to xattrs in the
	// "user.*" namespace, but we don't allow any other xattr prefixes anyway.
	if !fs.IsRegular(i.StableAttr) && !fs.IsDir(i.StableAttr) {
		if perms.Write {
			return syserror.EPERM
		}
		return syserror.ENODATA
	}

	return i.CheckPermission(t, perms)
}
