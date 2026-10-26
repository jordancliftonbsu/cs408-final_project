#!/bin/bash

echo "=== Running FantasyLoL Test Suite ==="

# Run unit and integration tests
echo "Running Go tests..."
if go test -v -short ./...; then
    echo "Go tests executed successfully."
else
    echo "Error: One or more Go tests failed!"
    exit 1
fi

echo "-------------------------------------"

# Check compilation
echo "Verifying build compilation..."
if go build -o /dev/null .; then
    echo "Application compiled successfully."
else
    echo "Error: Build compilation failed!"
    exit 1
fi

echo "====================================="
echo "ALL VERIFICATIONS PASSED! Ready for deployment."
echo "====================================="