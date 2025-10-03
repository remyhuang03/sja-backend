#!/bin/bash

# quit on err
set -e  # 遇到错误立即退出

echo "🚀 Starting SJA Backend..."

# check file .env 
if [ ! -f ".env" ]; then
    echo "❌ Error: .env not found"
    exit 1
fi

# Read .env
echo "📄 Loading env variables..."
while IFS='=' read -r key value; do
    # skip empty lines and comments
    [[ $key =~ ^[[:space:]]*# ]] && continue
    [[ -z $key ]] && continue

    # remove leading and trailing whitespace
    key=$(echo "$key" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
    value=$(echo "$value" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')

    # set env variables
    if [ -n "$key" ] && [ -n "$value" ]; then
        export "$key=$value"
        echo "  ✓ $key=$value"
    fi
done < .env

# Check Go installation
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go compiler not found, please install Go"
    exit 1
fi

echo "🔨 Building Go project..."
go mod tidy
go build -o sja-backend

# Check build success
if [ ! -f "sja-backend" ]; then
    echo "❌ Error: Build failed"
    exit 1
fi

echo "✅ Build succeeded!"

# Create logs directory if not exists
mkdir -p logs

# Get current timestamp for log file name
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
LOG_FILE="logs/sja-backend_$TIMESTAMP.log"

echo "🎯 Starting service..."
echo "📝 Log file: $LOG_FILE"
echo "🌐 Listening on port: $BACKEND_PORT"
echo ""


exec ./sja-backend >> "$LOG_FILE" 2>&1