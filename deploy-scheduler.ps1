$SERVER = "root@<your-server>"
$BINARY = "forge-agent-scheduler"
$REMOTE_BIN = "/usr/local/bin/$BINARY"

Write-Host "Building linux/amd64..."
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -o $BINARY ./cmd/scheduler
$env:GOOS = ""; $env:GOARCH = ""

if (-not $?) { Write-Host "Build failed"; exit 1 }

Write-Host "Stopping service..."
ssh $SERVER "systemctl stop $BINARY"

Write-Host "Uploading..."
scp $BINARY "${SERVER}:${REMOTE_BIN}"

Write-Host "Starting service..."
ssh $SERVER "chmod +x $REMOTE_BIN && systemctl start $BINARY && systemctl status $BINARY --no-pager"

Remove-Item $BINARY
Write-Host "Done."
