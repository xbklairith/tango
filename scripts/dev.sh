#!/bin/bash
cd /Users/xb/builder/ari
echo "Building frontend..."
make ui-build
echo "Starting server..."
exec make dev
