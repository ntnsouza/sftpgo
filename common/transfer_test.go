package common

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/drakkan/sftpgo/dataprovider"
	"github.com/drakkan/sftpgo/vfs"
)

func TestTransferUpdateQuota(t *testing.T) {
	conn := NewBaseConnection("", ProtocolSFTP, dataprovider.User{}, nil)
	transfer := BaseTransfer{
		Connection:    conn,
		transferType:  TransferUpload,
		BytesReceived: 123,
	}
	errFake := errors.New("fake error")
	transfer.TransferError(errFake)
	assert.False(t, transfer.updateQuota(1))
	err := transfer.Close()
	if assert.Error(t, err) {
		assert.EqualError(t, err, errFake.Error())
	}
	mappedPath := filepath.Join(os.TempDir(), "vdir")
	vdirPath := "/vdir"
	conn.User.VirtualFolders = append(conn.User.VirtualFolders, vfs.VirtualFolder{
		BaseVirtualFolder: vfs.BaseVirtualFolder{
			MappedPath: mappedPath,
		},
		VirtualPath: vdirPath,
		QuotaFiles:  -1,
		QuotaSize:   -1,
	})
	transfer.ErrTransfer = nil
	transfer.BytesReceived = 1
	transfer.requestPath = "/vdir/file"
	assert.True(t, transfer.updateQuota(1))
	err = transfer.Close()
	assert.NoError(t, err)
}

func TestTransferThrottling(t *testing.T) {
	u := dataprovider.User{
		Username:          "test",
		UploadBandwidth:   50,
		DownloadBandwidth: 40,
	}
	testFileSize := int64(131072)
	wantedUploadElapsed := 1000 * (testFileSize / 1000) / u.UploadBandwidth
	wantedDownloadElapsed := 1000 * (testFileSize / 1000) / u.DownloadBandwidth
	// some tolerance
	wantedUploadElapsed -= wantedDownloadElapsed / 10
	wantedDownloadElapsed -= wantedDownloadElapsed / 10
	conn := NewBaseConnection("id", ProtocolSCP, u, nil)
	transfer := NewBaseTransfer(nil, conn, nil, "", "", TransferUpload, 0, 0, true)
	transfer.BytesReceived = testFileSize
	transfer.Connection.UpdateLastActivity()
	startTime := transfer.Connection.GetLastActivity()
	transfer.HandleThrottle()
	elapsed := time.Since(startTime).Nanoseconds() / 1000000
	assert.GreaterOrEqual(t, elapsed, wantedUploadElapsed, "upload bandwidth throttling not respected")
	err := transfer.Close()
	assert.NoError(t, err)

	transfer = NewBaseTransfer(nil, conn, nil, "", "", TransferDownload, 0, 0, true)
	transfer.BytesSent = testFileSize
	transfer.Connection.UpdateLastActivity()
	startTime = transfer.Connection.GetLastActivity()

	transfer.HandleThrottle()
	elapsed = time.Since(startTime).Nanoseconds() / 1000000
	assert.GreaterOrEqual(t, elapsed, wantedDownloadElapsed, "download bandwidth throttling not respected")
	err = transfer.Close()
	assert.NoError(t, err)
}

func TestTransferErrors(t *testing.T) {
	isCancelled := false
	cancelFn := func() {
		isCancelled = true
	}
	testFile := filepath.Join(os.TempDir(), "transfer_test_file")
	fs := vfs.NewOsFs("id", os.TempDir(), nil)
	u := dataprovider.User{
		Username: "test",
		HomeDir:  os.TempDir(),
	}
	err := ioutil.WriteFile(testFile, []byte("test data"), os.ModePerm)
	assert.NoError(t, err)
	file, err := os.Open(testFile)
	if !assert.NoError(t, err) {
		assert.FailNow(t, "unable to open test file")
	}
	conn := NewBaseConnection("id", ProtocolSFTP, u, fs)
	transfer := NewBaseTransfer(file, conn, nil, testFile, "/transfer_test_file", TransferUpload, 0, 0, true)
	assert.Nil(t, transfer.cancelFn)
	assert.Equal(t, testFile, transfer.GetFsPath())
	transfer.SetCancelFn(cancelFn)
	errFake := errors.New("err fake")
	transfer.BytesReceived = 9
	transfer.TransferError(ErrQuotaExceeded)
	assert.True(t, isCancelled)
	transfer.TransferError(errFake)
	assert.Error(t, transfer.ErrTransfer, ErrQuotaExceeded.Error())
	// the file is closed from the embedding struct before to call close
	err = file.Close()
	assert.NoError(t, err)
	err = transfer.Close()
	if assert.Error(t, err) {
		assert.Error(t, err, ErrQuotaExceeded.Error())
	}
	assert.NoFileExists(t, testFile)

	err = ioutil.WriteFile(testFile, []byte("test data"), os.ModePerm)
	assert.NoError(t, err)
	file, err = os.Open(testFile)
	if !assert.NoError(t, err) {
		assert.FailNow(t, "unable to open test file")
	}
	fsPath := filepath.Join(os.TempDir(), "test_file")
	transfer = NewBaseTransfer(file, conn, nil, fsPath, "/test_file", TransferUpload, 0, 0, true)
	transfer.BytesReceived = 9
	transfer.TransferError(errFake)
	assert.Error(t, transfer.ErrTransfer, errFake.Error())
	// the file is closed from the embedding struct before to call close
	err = file.Close()
	assert.NoError(t, err)
	err = transfer.Close()
	if assert.Error(t, err) {
		assert.Error(t, err, errFake.Error())
	}
	assert.NoFileExists(t, testFile)

	err = ioutil.WriteFile(testFile, []byte("test data"), os.ModePerm)
	assert.NoError(t, err)
	file, err = os.Open(testFile)
	if !assert.NoError(t, err) {
		assert.FailNow(t, "unable to open test file")
	}
	transfer = NewBaseTransfer(file, conn, nil, fsPath, "/test_file", TransferUpload, 0, 0, true)
	transfer.BytesReceived = 9
	// the file is closed from the embedding struct before to call close
	err = file.Close()
	assert.NoError(t, err)
	err = transfer.Close()
	assert.NoError(t, err)
	assert.NoFileExists(t, testFile)
	assert.FileExists(t, fsPath)
	err = os.Remove(fsPath)
	assert.NoError(t, err)

	assert.Len(t, conn.GetTransfers(), 0)
}
