#!/bin/bash
set -e
echo "Building frontend for production..."
cd panel
npm run build

echo "Starting production server..."
npm start
