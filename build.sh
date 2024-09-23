#!/bin/bash

# Set the output binary name
OUTPUT_BINARY="SN-wallet"

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

    # Check if the user has provided a path to copy the binary
    if [ ! -z "$1" ]; then
        # Ensure the provided path exists
        if [ -d "$1" ]; then
            DESTINATION="$1/$OUTPUT_BINARY"
            
            # Remove any previous binary in the destination path
            if [ -f "$DESTINATION" ]; then
                echo "Removing existing binary at: $DESTINATION"
                rm -f "$DESTINATION"
            fi

            # Copy the new binary to the destination
            echo "Copying the binary to the provided path: $1"
            cp $OUTPUT_BINARY "$1/"
            echo "Binary copied to $1"
        else
            echo "The provided path does not exist: $1"
            exit 1
        fi
    fi
else
    echo "Build failed!"
    exit 1
fi