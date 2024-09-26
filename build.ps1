# Set the output binary name
$OUTPUT_BINARY = "SN-wallet.exe"

# Set the main package or the entry point for your Go application
$MAIN_PACKAGE = "./cmd/wallet"

# Clean previous builds
Write-Host "Cleaning previous builds..."
if (Test-Path $OUTPUT_BINARY) {
    Remove-Item -Force $OUTPUT_BINARY
}

# Build the Go project
Write-Host "Building the Go project..."
$buildResult = go build -o $OUTPUT_BINARY $MAIN_PACKAGE

# Check if the build was successful
if ($LASTEXITCODE -eq 0) {
    Write-Host "Build successful! Binary created: $OUTPUT_BINARY"

    # Check if the user has provided a path to copy the binary
    if ($args.Length -gt 0) {
        $destinationPath = $args[0]

        # Ensure the provided path exists
        if (Test-Path $destinationPath) {
            $destination = Join-Path $destinationPath $OUTPUT_BINARY

            # Remove any previous binary in the destination path
            if (Test-Path $destination) {
                Write-Host "Removing existing binary at: $destination"
                Remove-Item -Force $destination
            }

            # Copy the new binary to the destination
            Write-Host "Copying the binary to the provided path: $destinationPath"
            Copy-Item $OUTPUT_BINARY -Destination $destinationPath
            Write-Host "Binary copied to $destinationPath"
        } else {
            Write-Host "The provided path does not exist: $destinationPath"
            exit 1
        }
    }
} else {
    Write-Host "Build failed!"
    exit 1
}
