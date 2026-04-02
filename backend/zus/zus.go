package zus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"bytes"
	"io"
	"log"
	"sync"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/0chain/gosdk/constants"
	"github.com/0chain/gosdk/core/client"
	"github.com/0chain/gosdk/core/conf"
	"github.com/0chain/gosdk/zboxcore/fileref"
	"github.com/0chain/gosdk/zboxcore/sdk"
	"github.com/0chain/gosdk/zcncore"
	"github.com/mitchellh/go-homedir"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/configstruct"
	"github.com/rclone/rclone/fs/hash"
	"github.com/rclone/rclone/lib/batcher"
	"github.com/rclone/rclone/lib/encoder"
)

var (
	// walletMu serializes wallet activation across multiple Fs instances.
	// The gosdk uses global wallet state for signing blobber requests.
	// When multiple remotes are open (cross-allocation transfers), we must
	// switch the active wallet before each operation.
	walletMu sync.Mutex

	// sdkInitialized tracks whether InitSDK has been called (only needed once)
	sdkInitialized bool

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
	AllocationID string               `config:"allocation_id"`
	ConfigDir    string               `config:"config_dir"`
	Encrypt      bool                 `config:"encrypt"`
	WorkDir      string               `config:"work_dir"`
	SdkLogLevel  int                  `config:"sdk_log_level"`
	BatchMode    string               `config:"batch_mode"`
	BatchTimeout time.Duration        `config:"batch_timeout"`
	BatchSize    int                  `config:"batch_size"`
	Enc          encoder.MultiEncoder `config:"encoding"`
}

type Fs struct {
	name string //name of the remote
	root string //root of the remote

	opts            Options      // parsed options
	features        *fs.Features // optional features
	alloc           *sdk.Allocation
	batcher         *batcher.Batcher[sdk.OperationRequest, struct{}] // batcher for operations
	walletInfo      string // wallet JSON for this remote
	signatureScheme string // signature scheme for this remote
}

// activateWallet switches the gosdk's global wallet state to this Fs's wallet.
// Must be called before any blobber operation. Call deactivateWallet() when done.
func (f *Fs) activateWallet() {
	walletMu.Lock()
	_ = zcncore.SetGeneralWalletInfo(f.walletInfo, f.signatureScheme)
}

// deactivateWallet releases the wallet mutex.
func (f *Fs) deactivateWallet() {
	walletMu.Unlock()
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
			{
				Name:     config.ConfigEncoding,
				Help:     config.ConfigEncodingHelp,
				Advanced: true,
				Default: (encoder.MultiEncoder)(
					encoder.EncodeInvalidUtf8 |
						encoder.EncodeCtl |
						encoder.EncodeDel |
						encoder.EncodeDot |
						encoder.EncodeSlash |
						encoder.EncodePercent |
						encoder.EncodeCrLf |
						encoder.EncodeLeftSpace |
						encoder.EncodeLeftTilde |
						encoder.EncodeLeftCrLfHtVt |
						encoder.EncodeLeftPeriod |
						encoder.EncodeRightSpace |
						encoder.EncodeRightCrLfHtVt |
						encoder.EncodeRightPeriod),
			},
		}, defaultBatcherOptions.FsOptions("zus")...),
	})
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

// ensureParentDirs creates all parent directories for the given path if they don't exist.
// The SDK does not auto-create intermediate directories during upload.
func (f *Fs) ensureParentDirs(remotepath string) error {
	dir := path.Dir(remotepath)
	if dir == "/" || dir == "." || dir == "" {
		return nil
	}

	// Check if directory already exists
	level := len(strings.Split(strings.TrimSuffix(dir, "/"), "/"))
	oResult, err := f.alloc.GetRefs(dir, "", "", "", "", "regular", level, 1)
	if err == nil && len(oResult.Refs) > 0 && oResult.Refs[0].Type == fileref.DIRECTORY {
		return nil // Directory exists
	}

	// Create the directory (SDK handles creating intermediates with PreservePath)
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationCreateDir,
		RemotePath:    dir,
		PreservePath:  true,
	}
	return f.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
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
	f.walletInfo = walletInfo
	f.signatureScheme = cfg.SignatureScheme

	if !sdkInitialized {
		err = client.InitSDK("{}", cfg.BlockWorker, cfg.ChainID, cfg.SignatureScheme, 0, true, cfg.MinSubmit, cfg.MinConfirmation, cfg.ConfirmationChainLength, cfg.SharderConsensous)
		if err != nil {
			return nil, err
		}
		conf.InitClientConfig(&cfg)
		sdk.SetNumBlockDownloads(100)
		sdk.SetSaveProgress(false)
		sdk.SetLogLevel(f.opts.SdkLogLevel)
		sdkInitialized = true
	}

	err = zcncore.SetGeneralWalletInfo(walletInfo, cfg.SignatureScheme)
	if err != nil {
		return nil, err
	}

	if client.GetClient().IsSplit {
		zcncore.RegisterZauthServer(cfg.ZauthServer)
	}
	allocation, err := sdk.GetAllocation(f.opts.AllocationID)
	if err != nil {
		return nil, err
	}
	f.alloc = allocation

	// Check if root points to a file (rclone convention: return parent dir + fs.ErrorIsFile)
	if f.root != "/" {
		level := len(strings.Split(strings.TrimSuffix(f.root, "/"), "/"))
		oResult, refErr := f.alloc.GetRefs(f.root, "", "", "", "", "regular", level, 1)
		if refErr == nil && len(oResult.Refs) > 0 && oResult.Refs[0].Type != fileref.DIRECTORY {
			f.root = path.Dir(f.root)
			return f, fs.ErrorIsFile
		}
	}

	return f, nil
}

func (f *Fs) Equal(fs2 fs.Fs) bool {
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

// Features returns the optional features of this Fs
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
	f.activateWallet()
	defer f.deactivateWallet()
	return f.list(ctx, dir)
}

// list is the internal version that does not lock the wallet mutex.
func (f *Fs) list(ctx context.Context, dir string) (entries fs.DirEntries, err error) {
	encodedDir := f.opts.Enc.FromStandardPath(dir)
	remotepath := path.Join(f.root, encodedDir)
	fs.Debug("List: ", remotepath)

	// Normalize and construct the full remote path
	if f.root == "" && (dir == "" || dir == ".") {
		remotepath = "/"
	} else {
		remotepath = path.Join("/", f.root, encodedDir)
	}

	remotepath = path.Clean(remotepath)

	// Calculate the directory depth level by counting '/' segments
	level := len(strings.Split(remotepath, "/"))

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
			relPath := f.opts.Enc.ToStandardPath(trimLeadingPath(child.Path, f.root))
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
	f.activateWallet()
	defer f.deactivateWallet()
	return f.newObject(ctx, remote)
}

// newObject is the internal version of NewObject that does not lock the wallet mutex.
// Use when the caller already holds the lock.
func (f *Fs) newObject(ctx context.Context, remote string) (fs.Object, error) {
	remote = strings.TrimPrefix(remote, "/")
	remotepath := path.Join(f.root, f.opts.Enc.FromStandardPath(remote))
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
	f.activateWallet()
	defer f.deactivateWallet()

	for _, option := range options {
		if option.Mandatory() {
			fs.Errorf(f, "Unsupported mandatory option: %v", option)
			return nil, errors.New("unsupported mandatory option")
		}
	}
	remotepath := path.Join(f.root, f.opts.Enc.FromStandardPath(src.Remote()))
	obj := &Object{
		fs:     f,
		remote: remotepath,
	}

	// Check if the file already exists; if so, update it instead of inserting
	// Use internal update to avoid deadlock (wallet lock already held by Put)
	existing, err := f.newObject(ctx, src.Remote())
	if err == nil {
		return existing, existing.(*Object).update(ctx, in, src, options...)
	}

	err = obj.put(ctx, in, src, false)
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
	f.activateWallet()
	defer f.deactivateWallet()

	remotepath := path.Join(f.root, f.opts.Enc.FromStandardPath(dir))
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
	f.activateWallet()
	defer f.deactivateWallet()

	remotepath := path.Join(f.root, f.opts.Enc.FromStandardPath(dir))
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
func (f *Fs) Purge(ctx context.Context, dir string) error {
	f.activateWallet()
	defer f.deactivateWallet()

	remotepath := path.Join(f.root, f.opts.Enc.FromStandardPath(dir))
	if remotepath == "" || remotepath == "." {
		remotepath = f.root
	}
	level := len(strings.Split(strings.TrimSuffix(remotepath, "/"), "/"))
	oREsult, err := f.alloc.GetRefs(remotepath, "", "", "", "", "regular", level, 1)
	if err != nil {
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
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return fs.ErrorDirNotFound
		}
		return err
	}

	// Verify deletion — second purge should return ErrorDirNotFound
	oREsult, err = f.alloc.GetRefs(remotepath, "", "", "", "", "regular", level, 1)
	if err == nil && len(oREsult.Refs) > 0 && oREsult.Refs[0].Type == fileref.DIRECTORY {
		// Directory still appears (eventual consistency); try delete again
		_ = f.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
	}

	return nil
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
func trimLeadingPath(fullPath, root string) string {
	root = path.Clean("/" + root)
	fullPath = path.Clean(fullPath)

	if root == "/" {
		return strings.TrimPrefix(fullPath, "/")
	}
	return strings.TrimPrefix(fullPath, root+"/")
}

// Remote returns the object's path relative to the backend's configured root.
func (o *Object) Remote() string {
	return o.fs.opts.Enc.ToStandardPath(trimLeadingPath(o.remote, o.fs.root))
}

// ModTime returns the modification date of the file
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

// MimeType returns the content type of the Object if known
func (o *Object) MimeType(ctx context.Context) string {
	return o.mimeType
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
	// For zero-length files (stored as 32-byte hash), return empty reader
	if o.size == 0 {
		return io.NopCloser(strings.NewReader("")), nil
	}

	// Buffer the entire download while holding the wallet lock.
	// The SDK's download goroutine signs chunk requests using the active wallet,
	// so we must keep our wallet active until the download completes.
	o.fs.activateWallet()
	reader, err := o.fs.alloc.DownloadObject(ctx, o.remote, rangeStart, rangeEnd)
	if err != nil {
		o.fs.deactivateWallet()
		return nil, err
	}
	data, err := io.ReadAll(reader)
	reader.Close()
	o.fs.deactivateWallet()
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Update the object with the contents of the io.Reader, modTime and size
func (o *Object) Update(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (err error) {
	o.fs.activateWallet()
	defer o.fs.deactivateWallet()
	return o.update(ctx, in, src, options...)
}

// update is the internal version that does not lock the wallet mutex.
func (o *Object) update(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (err error) {
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
		if strings.Contains(err.Error(), "No data to upload") {
			return fs.ErrorCantUploadEmptyFiles
		}
		return err
	}
	o.modTime = modified
	o.size = rb.size
	o.encrypted = o.fs.opts.Encrypt

	// Refresh metadata from server to get correct hash and mime type
	return o.readMetaData(ctx)
}

func (o *Object) put(ctx context.Context, in io.Reader, src fs.ObjectInfo, toUpdate bool) (err error) {
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
		// Peek the reader: if empty, use non-stream mode (SDK handles zero-byte non-stream uploads)
		peekBuf := make([]byte, 1)
		n, peekErr := in.Read(peekBuf)
		if n == 0 && (peekErr == io.EOF || peekErr == nil) {
			isStreamUpload = false
		} else if n > 0 {
			in = io.MultiReader(bytes.NewReader(peekBuf[:n]), in)
		}
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

	// Ensure parent directories exist (SDK does not auto-create them)
	if !toUpdate {
		if err := o.fs.ensureParentDirs(o.remote); err != nil {
			log.Printf("Warning: failed to create parent dirs for %s: %v", o.remote, err)
		}
	}

	// If the batcher is enabled, we commit the operation through the batcher
	if o.fs.batcher.Batching() {
		_, err = o.fs.batcher.Commit(ctx, o.remote, opRequest)
	} else {
		err = o.fs.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
	}

	if err != nil {
		// SDK returns "No data to upload" for zero-byte streams
		if strings.Contains(err.Error(), "No data to upload") {
			return fs.ErrorCantUploadEmptyFiles
		}
		log.Printf("Failed to upload to %s: %v", o.remote, err)
		return err
	}

	o.modTime = modified
	o.size = rb.size
	o.encrypted = o.fs.opts.Encrypt
	o.mimeType = fileMeta.MimeType
	return nil
}

// Remove an object
func (o *Object) Remove(ctx context.Context) (err error) {
	o.fs.activateWallet()
	defer o.fs.deactivateWallet()
	return o.remove(ctx)
}

// remove is the internal version that does not lock the wallet mutex.
func (o *Object) remove(ctx context.Context) (err error) {
	opRequest := sdk.OperationRequest{
		OperationType: constants.FileOperationDelete,
		RemotePath:    o.remote,
		PreservePath:  true,
	}

	if o.fs == nil || o.fs.alloc == nil {
		return errors.New("filesystem not initialized")
	}

	if o.fs.batcher.Batching() {
		_, err = o.fs.batcher.Commit(ctx, o.remote, opRequest)
	} else {
		err = o.fs.alloc.DoMultiOperation([]sdk.OperationRequest{opRequest})
	}
	if err != nil {
		log.Printf("Failed to remove %s: %v", o.remote, err)
	}
	return err
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
	if ref.Type == fileref.DIRECTORY {
		return fs.ErrorIsDir
	}
	modTime := ref.UpdatedAt.ToTime()
	mp := make(map[string]string)
	if ref.CustomMeta != "" {
		err = json.Unmarshal([]byte(ref.CustomMeta), &mp)
		if err != nil {
			return err
		}
		t, ok := mp["rclone:mtime"]
		if ok {
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

	// SDK stores 0-byte files as 32-byte MD5 hash string; report true size
	if o.md5 == empty_string_md5_hash || o.size == 0 {
		o.md5 = empty_string_md5_hash
		o.size = 0
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

	// SDK stores 0-byte files as 32-byte MD5 hash string; report true size
	if o.md5 == empty_string_md5_hash || o.size == 0 {
		o.md5 = empty_string_md5_hash
		o.size = 0
	}
	return nil
}

// Note: Server-side Copy and Move are not implemented because the Züs SDK's
// copy operation does not support overwriting existing files, and move has
// consistency issues with path resolution.
// rclone will automatically fall back to download+re-upload for copy,
// and copy+delete for move.

// ListR lists the objects and directories of the Fs starting from dir recursively.
func (f *Fs) ListR(ctx context.Context, dir string, callback fs.ListRCallback) error {
	f.activateWallet()
	defer f.deactivateWallet()
	return f.listR(ctx, dir, callback)
}

func (f *Fs) listR(ctx context.Context, dir string, callback fs.ListRCallback) error {
	entries, err := f.list(ctx, dir)
	if err != nil {
		return err
	}
	var dirs []string
	for _, entry := range entries {
		if d, ok := entry.(fs.Directory); ok {
			dirs = append(dirs, d.Remote())
		}
	}
	err = callback(entries)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		err = f.listR(ctx, d, callback)
		if err != nil {
			return err
		}
	}
	return nil
}

// Note: DirMove is not implemented because the Züs SDK's directory move
// operation can leave the allocation's write marker in an inconsistent state.
// rclone will automatically fall back to file-by-file copy+delete.

// About gets quota information from the Fs
func (f *Fs) About(ctx context.Context) (*fs.Usage, error) {
	total := f.alloc.Size
	var used int64
	stats := f.alloc.GetStats()
	if stats != nil {
		used = stats.UsedSize
	}
	free := total - used
	return &fs.Usage{
		Total: &total,
		Used:  &used,
		Free:  &free,
	}, nil
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

// Check interfaces
var (
	_ fs.Fs        = (*Fs)(nil)
	_ fs.Purger  = (*Fs)(nil)
	_ fs.ListRer = (*Fs)(nil)
	_ fs.Abouter = (*Fs)(nil)
	_ fs.Object    = (*Object)(nil)
	_ fs.MimeTyper = (*Object)(nil)
)
