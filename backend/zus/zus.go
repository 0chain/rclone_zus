package zus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/0chain/gosdk/constants"
	"github.com/0chain/gosdk/core/client"
	"github.com/0chain/gosdk/core/conf"
	"github.com/0chain/gosdk/zboxcore/fileref"
	"github.com/0chain/gosdk/zboxcore/sdk"
	"github.com/0chain/gosdk/zcncore"
	"github.com/mitchellh/go-homedir"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/configstruct"
	"github.com/rclone/rclone/fs/hash"
	"github.com/rclone/rclone/lib/batcher"
)

var (
	// batcher default options
	defaultBatcherOptions = batcher.Options{
		MaxBatchSize:          50,
		DefaultTimeoutSync:    500 * time.Millisecond,
		DefaultTimeoutAsync:   5 * time.Second,
		DefaultBatchSizeAsync: 100,
	}
)

const (
	empty_string_md5_hash = "d41d8cd98f00b204e9800998ecf8427e"
)

type Options struct {
	AllocationID string        `config:"allocation_id"`
	ConfigDir    string        `config:"config_dir"`
	Encrypt      bool          `config:"encrypt"`
	WorkDir      string        `config:"work_dir"`
	SdkLogLevel  int           `config:"sdk_log_level"`
	BatchMode    string        `config:"batch_mode"`
	BatchTimeout time.Duration `config:"batch_timeout"`
	BatchSize    int           `config:"batch_size"`
}

type Fs struct {
	name string //name of the remote
	root string //root of the remote

	opts     Options      // parsed options
	features *fs.Features // optional features
	alloc    *sdk.Allocation
	batcher  *batcher.Batcher[sdk.OperationRequest, struct{}] // batcher for operations
}

func init() {
	fs.Register(&fs.RegInfo{
		Name:        "zus",
		Description: "Zus Decentralized Storage",
		NewFs:       NewFs,
		Options: append([]fs.Option{
			{
				Name: "allocation_id",
				Help: "Allocation ID to use for this remote",
			},
			{
				Name:    "config_dir",
				Help:    "Directory where the configuration files are stored",
				Default: nil,
			},
			{
				Name:    "work_dir",
				Help:    "Directory where the work files are stored",
				Default: nil,
			},
			{
				Name:    "encrypt",
				Help:    "Encrypt the data before uploading",
				Default: false,
			},
			{
				Name:     "sdk_log_level",
				Help:     "Log level for the SDK",
				Default:  0,
				Advanced: true,
			},
		}, defaultBatcherOptions.FsOptions("zus")...),
	})
}

// Validates if the path is a valid UTF-8 string
func isValidUTF8Path(path string) bool {
	return utf8.ValidString(path)
}

// removes newlines, tab spaces and extra unecessary
func removeWhitespace(r rune) rune {
	switch r {
	case ' ', '\n', '\r', '\t':
		return -1
	default:
		return r
	}
}

// NewFs constructs an Fs from the path
func NewFs(ctx context.Context, name, root string, m configmap.Mapper) (fs.Fs, error) {

	if root == "" {
		root = "/"
	}
	fs.Debug("root: ", root)
	if root[0] != '/' {
		root = "/" + root
	}
	root = path.Clean(root)

	f := &Fs{
		name: name,
		root: root,
	}

	// Parse config into Options struct
	err := configstruct.Set(m, &f.opts)
	if err != nil {
		return nil, err
	}

	f.features = (&fs.Features{
		CanHaveEmptyDirectories: true,
		ReadMimeType:            true,
		WriteMimeType:           true,
	}).Fill(ctx, f)

	batcherOptions := defaultBatcherOptions
	batcherOptions.Mode = f.opts.BatchMode
	batcherOptions.Size = f.opts.BatchSize
	batcherOptions.Timeout = f.opts.BatchTimeout
	f.batcher, err = batcher.New(ctx, f, f.commitBatch, batcherOptions)
	if err != nil {
		return nil, err
	}

	if f.opts.ConfigDir == "" {
		f.opts.ConfigDir, err = getDefaultConfigDir()
		if err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(f.opts.ConfigDir); err != nil {
		return nil, err
	}

	if f.opts.WorkDir == "" {
		f.opts.WorkDir, err = homedir.Dir()
		if err != nil {
			return nil, err
		}
	}

	if f.opts.AllocationID == "" {
		allocFile := filepath.Join(f.opts.ConfigDir, "allocation.txt")
		allocBytes, err := os.ReadFile(allocFile)
		if err != nil {
			return nil, err
		}

		// removes extra spaces and new lines from the allocation.txt if present
		allocationID := strings.Map(removeWhitespace, string(allocBytes))

		if len(allocationID) != 64 {
			return nil, fmt.Errorf("allocation id has length %d, should be 64", len(allocationID))
		}
		f.opts.AllocationID = allocationID
	}

	cfg, err := conf.LoadConfigFile(filepath.Join(f.opts.ConfigDir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	var walletInfo string
	walletFile := filepath.Join(f.opts.ConfigDir, "wallet.json")

	walletBytes, err := os.ReadFile(walletFile)
	if err != nil {
		return nil, err
	}
	walletInfo = string(walletBytes)
	err = client.InitSDK("{}", cfg.BlockWorker, cfg.ChainID, cfg.SignatureScheme, 0, true, cfg.MinSubmit, cfg.MinConfirmation, cfg.ConfirmationChainLength, cfg.SharderConsensous)
	if err != nil {
		return nil, err
	}
	conf.InitClientConfig(&cfg)

	err = zcncore.SetGeneralWalletInfo(walletInfo, cfg.SignatureScheme)
	if err != nil {
		return nil, err
	}

	if client.GetClient().IsSplit {
		zcncore.RegisterZauthServer(cfg.ZauthServer)
	}
	sdk.SetNumBlockDownloads(100)
	sdk.SetSaveProgress(false)
	sdk.SetLogLevel(f.opts.SdkLogLevel)
	allocation, err := sdk.GetAllocation(f.opts.AllocationID)
	if err != nil {
		return nil, err
	}
	f.alloc = allocation
	return f, nil
}

func (f *Fs) Equal(fs2 fs.Fs) bool {
	fmt.Println(">>> Equal() called")
	other, ok := fs2.(*Fs)
	if !ok {
		return false
	}
	return f.name == other.name && f.opts.AllocationID == other.opts.AllocationID
}

// Name of the remote (as passed into NewFs)
func (f *Fs) Name() string {
	return f.name
}

// Root of the remote (as passed into NewFs)
func (f *Fs) Root() string {
	//strip the leading / if present: "/root-name" --> "root-name"
	return strings.TrimPrefix(f.root, "/")
}

// String returns a description of the FS
func (f *Fs) String() string {
	return fmt.Sprintf("FS zus:%s", f.root)
}

// Precision of the ModTimes in this Fs
func (f *Fs) Precision() time.Duration {
	return time.Second
}

// Hashes are not exposed anywhere
func (f *Fs) Hashes() hash.Set {
	return hash.Set(hash.MD5)
}

// Features returns the optional features of thxis Fs
func (f *Fs) Features() *fs.Features {
	return f.features
}

// List the objects and directories in dir into entries.  The
// entries can be returned in any order but should be for a
// complete directory.
//
// dir should be "" to list the root, and should not have
// trailing slashes.
//
// This should return ErrDirNotFound if the directory isn't
// found.
func (f *Fs) List(ctx context.Context, dir string) (entries fs.DirEntries, err error) {
	remotepath := path.Join(f.root, dir)
	fs.Debug("List: ", remotepath)

	// Normalize and construct the full remote path
	if f.root == "" && (dir == "" || dir == ".") {
		// Special case: both root and dir are empty or current directory (".")
		// listing the top-level (root) directory
		remotepath = "/"
	} else {
		// Construct the full path by joining root and dir
		// Ensures path always starts from root (absolute)
		remotepath = path.Join("/", f.root, dir)
	}

	// Clean the path to remove redundant elements like multiple slashes or ".."
	remotepath = path.Clean(remotepath)

	// Calculate the directory depth level by counting '/' segments
	level := len(strings.Split(remotepath, "/"))

	// Special case: if remote path is exactly "/", set level to 1
	if remotepath == "/" {
		level = 1
	}

	oREsult, err := f.alloc.GetRefs(remotepath, "", "", "", "", "regular", level, 1)
	if err != nil {
		return nil, err
	}
	if len(oREsult.Refs) == 0 {
		return nil, fs.ErrorDirNotFound
	}

	ref := oREsult.Refs[0]

	// If the path is a file (not directory), return it directly
	if ref.Type != fileref.DIRECTORY {
		o := &Object{
			fs: f,
		}
		err = o.readFromRef(&ref)
		if err != nil {
			return nil, err
		}
		entries = append(entries, o)
		return entries, nil
	}

	// Otherwise, list directory contents
	res := f.alloc.ListObjects(ctx, remotepath, "", "", "", "", "regular", level+1, 1000)

	for child := range res {
		var entry fs.DirEntry
		if child.Err != nil {
			return nil, child.Err
		}
		if child.Type == fileref.DIRECTORY {
			// Handle subdirectory
			relPath := trimLeadingPath(child.Path, f.root)
			entry = fs.NewDir(relPath, child.UpdatedAt.ToTime())
		} else {
			o := &Object{
				fs: f,
			}
			err = o.readFromRef(&child)
			if err != nil {
				return nil, err
			}
			entry = o
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// NewObject finds the Object at remote.  If it can't be found
// it returns the error fs.ErrorObjectNotFound.
func (f *Fs) NewObject(ctx context.Context, remote string) (fs.Object, error) {
	remote = strings.TrimPrefix(remote, "/")
	remotepath := path.Join(f.root, remote)
	fs.Debug("NewObject: ", remotepath)
	o := &Object{
		fs:     f,
		remote: remotepath,
	}
	err := o.readMetaData(ctx)
	if err != nil {
		return nil, err
	}
	return o, nil
}

// Put the object
//
// Copy the reader in to the new object which is returned.
//
// The new object may have been created if an error is returned
func (f *Fs) Put(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	for _, option := range options {
		if option.Mandatory() {
			fs.Errorf(f, "Unsupported mandatory option: %v", option)

			return nil, errors.New("unsupported mandatory option")
		}
	}
	remotepath := path.Join(f.root, src.Remote())
	obj := &Object{
		fs:     f,
		remote: remotepath,
	}
	err := obj.put(ctx, in, src, false)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// PutStream uploads to the remote path with the modTime given of indeterminate size
func (f *Fs) PutStream(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	return f.Put(ctx, in, src, options...)
}

// Mkdir creates the directory if it doesn't exist
func (f *Fs) Mkdir(ctx context.Context, dir string) (err error) {
	remotepath := path.Join(f.root, dir)
	//Validate if the path is a valid UTF-8 string
	if !isValidUTF8Path(remotepath) {
		return fmt.Errorf("invalid UTF-8 characters in path: %s", remotepath)
	}
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationCreateDir,
		RemotePath:    remotepath,
		PreservePath:  true,
	}
	err = f.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
	return err
}

// Rmdir deletes the given folder
//
// Returns an error if it isn't empty
func (f *Fs) Rmdir(ctx context.Context, dir string) (err error) {
	remotepath := path.Join(f.root, dir)
	level := len(strings.Split(strings.TrimSuffix(remotepath, "/"), "/"))
	oREsult, err := f.alloc.GetRefs(remotepath, "", "", "", "", "regular", level, 1)
	if err != nil {
		return err
	}
	if len(oREsult.Refs) == 0 {
		return fs.ErrorDirNotFound
	}
	if oREsult.Refs[0].Type != fileref.DIRECTORY {
		return fs.ErrorDirNotFound
	}
	oREsult, err = f.alloc.GetRefs(remotepath, "", "", "", "", "regular", level+1, 1)
	if err != nil {
		return err
	}
	if len(oREsult.Refs) > 0 {
		return fs.ErrorDirectoryNotEmpty
	}
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationDelete,
		RemotePath:    remotepath,
		PreservePath:  true,
	}
	err = f.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
	return err
}

// Purge deletes all the files and the container
//
// Optional interface: Only implement this if you have a way of
// deleting all the files quicker than just running Remove() on the
// result of List()
func (f *Fs) Purge(ctx context.Context, dir string) error {
	remotepath := path.Join(f.root, dir)
	level := len(strings.Split(strings.TrimSuffix(remotepath, "/"), "/"))
	oREsult, err := f.alloc.GetRefs(remotepath, "", "", "", "", "regular", level, 1)
	if err != nil {
		// If the directory doesn't exist, we return an error
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
			return fs.ErrorDirNotFound
		}
		return err
	}
	if len(oREsult.Refs) == 0 {
		return fs.ErrorDirNotFound
	}
	if oREsult.Refs[0].Type != fileref.DIRECTORY {
		return fs.ErrorDirNotFound
	}
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationDelete,
		RemotePath:    remotepath,
		PreservePath:  true,
	}
	err = f.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})

	// After successful deletion, we should throw the DirNotFound error for purged directory
	if err != nil && strings.Contains(err.Error(), "not found") {
		return fs.ErrorDirNotFound
	}

	return err
}

type Object struct {
	fs        *Fs
	modTime   time.Time
	size      int64
	encrypted bool
	remote    string
	md5       string
	mimeType  string
}

// String returns a description of the Object
func (o *Object) String() string {
	if o == nil {
		return "<nil>"
	}

	return o.Remote()
}

// trimLeadingPath removes the leading root path from the full path to produce a relative path.
//
// Parameters:
//   - fullPath: the complete absolute path of the object
//   - root: the base path configured for the Fs instance
//
// Behavior:
//   - Ensures both fullPath and root are normalized using path.Clean()
//   - If root is "/", the function simply trims the leading "/" from fullPath
//   - If root is a subdirectory, it trims the root prefix + "/" from the fullPath
//
// Returns:
//   - A path relative to the root
func trimLeadingPath(fullPath, root string) string {
	// Ensure both root and fullPath are normalized and prefixed with a slash
	root = path.Clean("/" + root)
	fullPath = path.Clean(fullPath)

	if root == "/" {
		// If root is "/", trim leading slash only
		return strings.TrimPrefix(fullPath, "/")
	}
	// Remove the root prefix (plus one trailing slash) from the full path
	return strings.TrimPrefix(fullPath, root+"/")
}

// Remote returns the object’s path relative to the backend’s configured root.
//
// This is used by rclone to show the object's name/path from the user's perspective
// rather than the full internal absolute path.
func (o *Object) Remote() string {
	return trimLeadingPath(o.remote, o.fs.root)
}

// ModTime returns the modification date of the file
// It should return a best guess if one isn't available
func (o *Object) ModTime(ctx context.Context) time.Time {
	return o.modTime
}

// Size returns the size of the file
func (o *Object) Size() int64 {
	return o.size
}

// Fs returns read only access to the Fs that this object is part of
func (o *Object) Fs() fs.Info {
	return o.fs
}

// Hash returns the selected checksum of the file
// If no checksum is available it returns ""
func (o *Object) Hash(ctx context.Context, t hash.Type) (_ string, err error) {
	if t != hash.MD5 {
		return "", hash.ErrUnsupported
	}
	if o.md5 == "" {
		err = o.readMetaData(ctx)
		if err != nil {
			return "", err
		}
	}
	return o.md5, nil
}

// Storable says whether this object can be stored
func (o *Object) Storable() bool {
	return true
}

// SetModTime sets the metadata on the object to set the modification date
func (o *Object) SetModTime(ctx context.Context, t time.Time) (err error) {
	return fs.ErrorCantSetModTime
}

// Open an object for read
func (o *Object) Open(ctx context.Context, options ...fs.OpenOption) (io.ReadCloser, error) {
	var (
		rangeStart int64
		rangeEnd   int64 = -1
	)

	for _, option := range options {
		switch opt := option.(type) {
		case *fs.RangeOption:
			rangeStart = opt.Start
			rangeEnd = opt.End
		case *fs.SeekOption:
			if opt.Offset > 0 {
				rangeStart = opt.Offset
			} else {
				rangeStart = o.size + opt.Offset
			}
		default:
			if option.Mandatory() {
				fs.Errorf(o, "Unsupported mandatory option: %v", option)

				return nil, errors.New("unsupported mandatory option")
			}

		}
	}
	return o.fs.alloc.DownloadObject(ctx, o.remote, rangeStart, rangeEnd)
}

// Update the object with the contents of the io.Reader, modTime and size
//
// If existing is set then it updates the object rather than creating a new one.
//
// The new object may have been created if an error is returned.
func (o *Object) Update(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (err error) {
	for _, option := range options {
		if option.Mandatory() {
			fs.Errorf(o.fs, "Unsupported mandatory option: %v", option)

			return errors.New("unsupported mandatory option")
		}
	}
	mp := make(map[string]string)
	modified := src.ModTime(ctx)
	mp["rclone:mtime"] = modified.Format(time.RFC3339)
	marshal, err := json.Marshal(mp)
	if err != nil {
		return err
	}
	fileMeta := sdk.FileMeta{
		RemotePath: o.remote,
		ActualSize: src.Size(),
		RemoteName: path.Base(o.remote),
		CustomMeta: string(marshal),
	}
	isStreamUpload := src.Size() == -1
	if isStreamUpload {
		fileMeta.ActualSize = 0
	}
	rb := &ReaderBytes{
		reader: in,
	}
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationUpdate,
		FileReader:    rb,
		Workdir:       o.fs.opts.WorkDir,
		RemotePath:    o.remote,
		FileMeta:      fileMeta,
		Opts: []sdk.ChunkedUploadOption{
			sdk.WithChunkNumber(120),
			sdk.WithEncrypt(o.fs.opts.Encrypt),
		},
		StreamUpload: isStreamUpload,
		PreservePath: true,
	}
	err = o.fs.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
	if err != nil {
		return err
	}
	o.modTime = modified
	o.size = rb.size
	o.encrypted = o.fs.opts.Encrypt

	return nil
}

func (o *Object) put(ctx context.Context, in io.Reader, src fs.ObjectInfo, toUpdate bool) (err error) {
	// If the file size is 0, we return an error
	if !toUpdate && src.Size() == 0 {
		return fs.ErrorCantUploadEmptyFiles
	}

	//Validate if the path is a valid UTF-8 string
	if !isValidUTF8Path(o.remote) {
		return fmt.Errorf("invalid UTF-8 characters in path: %s", o.remote)
	}

	mp := make(map[string]string)
	modified := src.ModTime(ctx)
	mp["rclone:mtime"] = modified.Format(time.RFC3339)
	marshal, err := json.Marshal(mp)
	if err != nil {
		return err
	}
	fileMeta := sdk.FileMeta{
		Path:       "",
		RemotePath: o.remote,
		ActualSize: src.Size(),
		RemoteName: path.Base(o.remote),
		CustomMeta: string(marshal),
		MimeType:   fs.MimeType(ctx, src),
	}
	isStreamUpload := src.Size() == -1
	if isStreamUpload {
		fileMeta.ActualSize = 0
	}
	rb := &ReaderBytes{
		reader: in,
	}
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationInsert,
		FileReader:    rb,
		Workdir:       o.fs.opts.WorkDir,
		RemotePath:    o.remote,
		FileMeta:      fileMeta,
		Opts: []sdk.ChunkedUploadOption{
			sdk.WithChunkNumber(120),
			sdk.WithEncrypt(o.fs.opts.Encrypt),
		},
		StreamUpload: isStreamUpload,
		PreservePath: true,
	}
	if toUpdate {
		opRequest.OperationType = constants.FileOperationUpdate
	}

	// filesystem check
	if o.fs == nil || o.fs.alloc == nil {
		return errors.New("filesystem not initialized")
	}

	// If the batcher is enabled, we commit the operation through the batcher
	if o.fs.batcher.Batching() {
		_, err = o.fs.batcher.Commit(ctx, o.remote, opRequest)
	} else {
		err = o.fs.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
	}

	if err != nil {
		log.Printf("Failed to upload to %s: %v", o.remote, err)
		return err
	}

	o.modTime = modified
	o.size = rb.size
	o.encrypted = o.fs.opts.Encrypt
	o.mimeType = fileMeta.MimeType
	return nil
}

// Move moves a file from one location to another within the same remote.
//
// It constructs an SDK move operation, optionally batching it if the batcher is enabled.
// Returns a new object pointing to the destination if the move is successful.
// Move performs move operation using FileOpertaionMove of GoSDK: A metadata-only move, using blobber-native logic
func (f *Fs) Move(ctx context.Context, src fs.Object, remote string) (fs.Object, error) {
	// Type assert the source object to our backend-specific type
	srcObj, ok := src.(*Object)
	if !ok {
		return nil, errors.New("invalid object type")
	}

	// Construct absolute destination path
	dstPath := path.Join("/", f.root, remote) // e.g., /destination/file.extension
	dstDir := path.Dir(dstPath)               // e.g., /destination
	dstName := path.Base(dstPath)             // e.g., file.extension

	// Build the move operation request for the SDK
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationMove,
		RemotePath:    srcObj.remote, // Full source path, e.g., /directory/file.extension
		DestPath:      dstDir,        // Target directory path
		DestName:      dstName,       // Target file name
		PreservePath:  true,          // Preserve the original path of the file
	}

	// filesystem check
	if f == nil || f.alloc == nil {
		return nil, errors.New("filesystem not initialized")
	}

	var err error
	// If batching is enabled, defer execution via the batcher
	if f.batcher.Batching() {
		_, err = f.batcher.Commit(ctx, srcObj.remote, opRequest)
	} else {
		// Otherwise, perform the move operation immediately
		err = f.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
	}
	if err != nil {
		return nil, err
	}

	// Return a new Object representing the destination path
	newObj := &Object{
		fs:     f,
		remote: dstPath,
	}
	err = newObj.readMetaData(ctx)
	if err != nil {
		return nil, err
	}

	return newObj, nil
}

// Remove an object
func (o *Object) Remove(ctx context.Context) (err error) {
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationDelete,
		RemotePath:    o.remote,
		PreservePath:  true,
	}

	// filesystem check
	if o.fs == nil || o.fs.alloc == nil {
		return errors.New("filesystem not initialized")
	}

	// If batcher is enabled, we commit the operation through the batcher
	if o.fs.batcher.Batching() {
		_, err = o.fs.batcher.Commit(ctx, o.remote, opRequest)
		if err != nil {
			log.Printf("Failed to remove %s: %v", o.remote, err)
			return err
		}
	} else {
		err = o.fs.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
		if err != nil {
			log.Printf("Failed to remove %s: %v", o.remote, err)
			return err
		}
	}
	return nil
}

func (o *Object) readMetaData(ctx context.Context) (err error) {
	level := len(strings.Split(strings.TrimSuffix(o.remote, "/"), "/"))
	oREsult, err := o.fs.alloc.GetRefs(o.remote, "", "", "", "", "regular", level, 1)
	if err != nil {
		return err
	}
	if len(oREsult.Refs) == 0 {
		return fs.ErrorObjectNotFound
	}
	ref := oREsult.Refs[0]
	modTime := ref.UpdatedAt.ToTime()
	mp := make(map[string]string)
	if ref.CustomMeta != "" {
		err = json.Unmarshal([]byte(ref.CustomMeta), &mp)
		if err != nil {
			return err
		}
		t, ok := mp["rclone:mtime"]
		if ok {
			// try to parse the time
			tm, err := time.Parse(time.RFC3339, t)
			if err == nil {
				modTime = tm
			}
		}
	}
	o.modTime = modTime
	o.size = ref.ActualFileSize
	o.encrypted = ref.EncryptedKey != ""
	o.md5 = ref.ActualFileHash
	o.mimeType = ref.MimeType

	//If the file size is 0, we set the md5 to the default value
	if o.size == 0 {
		o.md5 = empty_string_md5_hash
	}
	return nil
}

func (o *Object) readFromRef(ref *sdk.ORef) error {
	mp := make(map[string]string)
	if ref.CustomMeta != "" {
		err := json.Unmarshal([]byte(ref.CustomMeta), &mp)
		if err != nil {
			return err
		}
	}
	modTime := ref.UpdatedAt.ToTime()
	t, ok := mp["rclone:mtime"]
	if ok {
		// try to parse the time
		tm, err := time.Parse(time.RFC3339, t)
		if err == nil {
			modTime = tm
		}
	}

	o.remote = ref.Path
	o.modTime = modTime
	o.size = ref.ActualFileSize
	o.encrypted = ref.EncryptedKey != ""
	o.md5 = ref.ActualFileHash
	o.mimeType = ref.MimeType

	//If the file size is 0, we set the md5 to the default value
	if o.size == 0 {
		o.md5 = empty_string_md5_hash
	}
	return nil
}

// Copy implements the fs.Fs Copy interface method.
// It performs a server-side copy of the source object to the specified destination path.
//
// Parameters:
//   - ctx: context for the operation (used for cancellation/deadlines)
//   - src: the source fs.Object to be copied (must be of type *Object)
//   - remote: the target relative path within the destination backend
//
// Returns:
//   - a new fs.Object representing the copied file
//   - an error if the operation fails
func (f *Fs) Copy(ctx context.Context, src fs.Object, remote string) (fs.Object, error) {
	// Type assert the source object to our custom Object type
	srcZus, ok := src.(*Object)
	if !ok {
		return nil, errors.New("invalid source object type")
	}

	// Build the full destination path from root and remote
	dstPath := path.Join("/", f.root, remote)
	dstPath = path.Clean(dstPath)

	// Extract directory and file name from the destination path
	dstDir := path.Dir(dstPath)
	dstName := path.Base(dstPath)

	// Construct the SDK operation request for copying
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationCopy,
		RemotePath:    srcZus.remote, // full source path from original Fs
		DestPath:      dstDir,
		DestName:      dstName,
		PreservePath:  true,
	}

	var err error
	// Use batcher if enabled, otherwise perform direct operation
	if f.batcher.Batching() {
		_, err = f.batcher.Commit(ctx, srcZus.remote, opRequest)
	} else {
		err = f.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
	}
	if err != nil {
		return nil, err
	}

	// Create and initialize the new Object representing the copied file
	newObj := &Object{
		fs:     f,
		remote: dstPath,
	}
	err = newObj.readMetaData(ctx)
	if err != nil {
		return nil, err
	}

	return newObj, nil
}

type ReaderBytes struct {
	reader io.Reader
	size   int64
}

func (r *ReaderBytes) Read(p []byte) (n int, err error) {
	n, err = r.reader.Read(p)
	r.size += int64(n)
	return n, err
}

func getDefaultConfigDir() (string, error) {
	homeDir, err := homedir.Dir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".zcn")

	return configDir, nil
}
