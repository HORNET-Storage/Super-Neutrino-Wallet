#!/bin/sh

WALLET_NAME=${WALLET_NAME:-"default"}
WALLET_PASSWORD=${WALLET_PASSWORD:-"password123"}

echo "Starting Bitcoin Wallet"

if ! ./wallet balance > /dev/null 2>&1; then
    echo "No wallet found, creating new wallet: $WALLET_NAME"
    ./wallet create "$WALLET_NAME" "$WALLET_PASSWORD" "" "" > /dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo "Failed to create wallet"
        exit 1
    fi
    echo "Wallet created successfully"
fi

echo "Starting wallet '$WALLET_NAME'"
exec ./wallet open "$WALLET_NAME" "$WALLET_PASSWORD"
