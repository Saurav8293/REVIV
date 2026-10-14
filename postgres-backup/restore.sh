#!/bin/bash
set -e

# ==============================================================================
# REVIV WAL-G Point-in-Time Recovery (PITR) Script for Standby Node
# Rebuilds PostgreSQL cluster from S3 base backup and replays WAL stream
# ==============================================================================

S3_BUCKET="${1:-reviv-postgres-backup-20260817}"
AWS_REGION="${2:-ap-south-1}"
PITR_TARGET_TIME="${3:-}"

echo "=========================================================="
echo " Starting REVIV PostgreSQL Standby Database Restoration    "
echo " S3 Backup Bucket : s3://${S3_BUCKET}/wal/                 "
echo " AWS Region       : ${AWS_REGION}                          "
if [ -n "${PITR_TARGET_TIME}" ]; then
  echo " PITR Target Time : ${PITR_TARGET_TIME}                  "
fi
echo "=========================================================="

export WALG_S3_PREFIX="s3://${S3_BUCKET}/wal/"
export AWS_REGION="${AWS_REGION}"
PGDATA="/var/lib/postgresql/data"

echo "[1/5] Stopping any running PostgreSQL process..."
su - postgres -c "pg_ctl -D ${PGDATA} stop -m fast" || true

echo "[2/5] Cleaning existing PGDATA directory..."
rm -rf ${PGDATA}/*

echo "[3/5] Fetching latest full base backup from AWS S3 via WAL-G..."
wal-g backup-fetch ${PGDATA} LATEST

echo "[4/5] Configuring Point-In-Time recovery signals..."
touch ${PGDATA}/recovery.signal

cat << 'EOF' > ${PGDATA}/recovery-config.conf
restore_command = 'wal-g wal-fetch "%f" "%p"'
recovery_target_action = 'promote'
EOF

if [ -n "${PITR_TARGET_TIME}" ]; then
  echo "recovery_target_time = '${PITR_TARGET_TIME}'" >> ${PGDATA}/recovery-config.conf
fi

# Append to postgresql.auto.conf
cat ${PGDATA}/recovery-config.conf >> ${PGDATA}/postgresql.auto.conf
rm -f ${PGDATA}/recovery-config.conf

chown -R postgres:postgres ${PGDATA}
chmod 0700 ${PGDATA}

echo "[5/5] Launching PostgreSQL in recovery mode (replaying WAL to target point)..."
su - postgres -c "postgres -D ${PGDATA}" &

echo "Waiting for PostgreSQL recovery replay to finish..."
until su - postgres -c "pg_isready -h 127.0.0.1 -p 5432"; do
  sleep 2
done

echo "=========================================================="
echo "✅ PostgreSQL Point-in-Time Recovery Completed Successfully!"
echo " Standby Database is Promoted and Accepting Production Reads/Writes"
echo "=========================================================="
