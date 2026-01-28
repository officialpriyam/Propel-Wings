#!/bin/bash
echo "Starting PropelPanel Environment..."

# Start Wings in background or new terminal usually, but here we'll just run them
# A simple way is to use screen or tmux, but simpler is just backgrounding one
echo "Starting Wings..."
(cd wings && ./propel --debug) &
WINGS_PID=$!

echo "Starting Panel (Development Mode)..."
cd panel
npm run dev

# cleanup
kill $WINGS_PID
