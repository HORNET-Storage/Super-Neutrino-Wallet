#!/bin/bash

# Set the output binary name
OUTPUT_BINARY="Nestr-wallet"

# Set the main package or the entry point for your Go application
MAIN_PACKAGE="./cmd/wallet"

# Clean previous builds
echo "Cleaning previous builds..."
rm -f $OUTPUT_BINARY

# Build the Go project
echo "Building the Go project..."
go build -o $OUTPUT_BINARY $MAIN_PACKAGE

# Check if the build was successful
if [ $? -eq 0 ]; then
    echo "Build successful! Binary created: $OUTPUT_BINARY"
else
    echo "Build failed!"
    exit 1
fi
