#!/bin/bash

# Process monitor script
# Starts a child process, monitors it, and restarts it if it exits
# Also properly passes signals to the child process

# Function to log messages with timestamps
log() {
	echo "[$(date '+%Y-%m-%d %H:%M:%S')]" "$@";
}

# Function to display usage information
show_usage() {
	cat << EOF
Usage: $0 [OPTIONS] COMMAND [ARGS...]

Starts, monitors, and automatically restarts a command if it exits.
Properly forwards signals to the monitored process.

Options:
  -h, --help              Show this help message and exit
  -d, --delay SECONDS     Initial delay before restarting, default: 1
  -m, --max-delay SECONDS Maximum restart delay, default: 60
  -f, --max-failures N    Number of failures before increasing delay, default: 5
  -w, --wait-child        Wait for child to exit after sending termination signal (default)
  -n, --no-wait-child     Don't wait for child to exit after sending termination signal
  -p, --pid-file FILE     Write monitor process PID to specified file

Examples:
  $0 my_server --port 8080              # Start and monitor 'my_server --port 8080'
  $0 -d 5 my_server                     # Use a 5-second initial restart delay
  $0 -n ruby app.rb                     # Don't wait for ruby to exit when terminating
  $0 -p /var/run/monitor.pid my_server  # Write monitor PID to specified file

EOF
}

# Default options
RESTART_DELAY="1";          # Initial delay before restarting
MAX_RESTART_DELAY="60";     # Maximum restart delay
MAX_FAILURES="5";           # Number of failures before increasing delay
WAIT_FOR_CHILD="true";      # Whether to wait for child to exit after termination signal
PID_FILE="";                # Path to file where to write the monitor's PID

# Parse command line options
while [[ $# -gt 0 ]]; do
	case "$1" in
		-h|--help)
			show_usage;
			exit 0;
			;;
		-d|--delay)
			RESTART_DELAY="$2";
			shift 2;
			;;
		-m|--max-delay)
			MAX_RESTART_DELAY="$2";
			shift 2;
			;;
		-f|--max-failures)
			MAX_FAILURES="$2";
			shift 2;
			;;
		-w|--wait-child)
			WAIT_FOR_CHILD="true";
			shift;
			;;
		-n|--no-wait-child)
			WAIT_FOR_CHILD="false";
			shift;
			;;
		-p|--pid-file)
			PID_FILE="$2";
			shift 2;
			;;
		--)
			shift;
			break;
			;;
		-*)
			log "Unknown option: $1";
			show_usage;
			exit 1;
			;;
		*)
			break;
			;;
	esac
done

# Check if arguments were provided
if [ $# -eq 0 ]; then
	show_usage;
	exit 1;
fi

# Initialize variables
CHILD_PID="";

# Write PID to file if specified
if [ -n "$PID_FILE" ]; then
	# Ensure the directory exists
	PID_DIR="$(dirname "$PID_FILE")";
	if ! mkdir -p "$PID_DIR" 2>/dev/null; then
		log "Error: Cannot create directory for PID file: $PID_DIR";
		exit 1;
	fi
	
	# Write PID to file
	if ! echo "$$" > "$PID_FILE"; then
		log "Error: Cannot write to PID file: $PID_FILE";
		exit 1;
	fi
	log "Monitor PID $$ written to file: $PID_FILE";
fi

# Function to clean up and exit
cleanup() {
	# Clean up child process
	if [ -n "$CHILD_PID" ] && kill -0 "$CHILD_PID" 2>/dev/null; then
		log "Terminating child process $CHILD_PID";
		kill -TERM "$CHILD_PID" 2>/dev/null;
		
		if [ "$WAIT_FOR_CHILD" = "true" ]; then
			log "Waiting for child process to exit...";
			wait "$CHILD_PID" 2>/dev/null;
		fi
	fi
	
	# Remove PID file if it exists
	if [ -n "$PID_FILE" ] && [ -f "$PID_FILE" ]; then
		rm -f "$PID_FILE";
		log "Removed PID file: $PID_FILE";
	fi
	
	log "Process monitor exiting";
	exit 0;
}

# Function to handle signals and forward them to the child process
handle_signal() {
	local sig="$1";
	log "Received signal: $sig";
	
	if [ -n "$CHILD_PID" ] && kill -0 "$CHILD_PID" 2>/dev/null; then
		log "Forwarding signal $sig to child process $CHILD_PID";
		kill -s "$sig" "$CHILD_PID";
	fi
	
	# For termination signals, we'll exit after handling the child
	if [ "$sig" = "TERM" ] || [ "$sig" = "INT" ] || [ "$sig" = "QUIT" ]; then
		cleanup;
	fi
}

# Set up signal handlers for common signals
trap 'handle_signal TERM' TERM;
trap 'handle_signal INT' INT;
trap 'handle_signal HUP' HUP;
trap 'handle_signal QUIT' QUIT;
trap 'handle_signal USR1' USR1;
trap 'handle_signal USR2' USR2;

# Function to start and monitor the child process
start_and_monitor_child() {
	log "Starting child process:" "$@";
	"$@" &
	CHILD_PID="$!";
	
	# Check if the process started successfully
	if kill -0 "$CHILD_PID" 2>/dev/null; then
		log "Child process started with PID: $CHILD_PID";
		
		# Wait for child to exit
		wait "$CHILD_PID";
		local exit_status="$?";
		log "Child process exited with status: $exit_status";
		CHILD_PID="";
		return "$exit_status";
	else
		log "Failed to start child process";
		CHILD_PID="";
		return 1;
	fi
}

log "Process monitor started";
log "Monitoring command:" "$@";

# Counter for consecutive failures
FAILURES="0";
current_delay="$RESTART_DELAY";

# Main loop to keep restarting the child process
while true; do
	start_and_monitor_child "$@";
	EXIT_STATUS="$?";
	
	# Increment failure counter if process exited with non-zero status
	if [ "$EXIT_STATUS" -ne 0 ]; then
		FAILURES="$((FAILURES + 1))";
		if [ "$FAILURES" -ge "$MAX_FAILURES" ]; then
			# Increase delay if we've had too many consecutive failures
			current_delay="$((current_delay * 2))";
			if [ "$current_delay" -gt "$MAX_RESTART_DELAY" ]; then
				current_delay="$MAX_RESTART_DELAY";  # Cap at max delay
			fi
			log "Too many consecutive failures. Increasing restart delay to $current_delay seconds.";
			FAILURES="0";
		fi
	else
		# Reset failure counter and delay if process exited cleanly
		FAILURES="0";
		current_delay="$RESTART_DELAY";
	fi
	
	log "Restarting child process in $current_delay seconds...";
	sleep "$current_delay";
done
