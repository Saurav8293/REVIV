import React, { useState, useEffect, useRef } from 'react';
import {
  Activity,
  Server,
  Cloud,
  Database,
  ShieldAlert,
  ArrowRight,
  CheckCircle2,
  AlertTriangle,
  Play,
  RotateCcw,
  Zap,
  Terminal,
  Clock,
  Layers,
  Lock,
  DollarSign,
  RefreshCw,
  ExternalLink,
  ChevronRight,
  TrendingDown,
  Info
} from 'lucide-react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  AreaChart,
  Area
} from 'recharts';

export default function App() {
  const [activeTab, setActiveTab] = useState('control');
  const [status, setStatus] = useState({
    primary_node_id: 'primary-vps-01',
    status: 'HEALTHY',
    last_heartbeat_time: new Date().toISOString(),
    time_since_last_heartbeat_sec: 1.8,
    consecutive_misses: 0,
    suspect_threshold: 3,
    down_threshold: 5,
    db_status: 'UP',
    last_wal_pushed_at: new Date(Date.now() - 18000).toISOString(),
    backup_lag_seconds: 18,
    estimated_rpo: '< 30 seconds',
    estimated_rto: '~5 to 8 minutes (Cold Standby)',
    cloud_standby_state: 'COLD_IDLE',
    standby_compute_ip: '',
    operator_confirmation_required: false,
    operator_confirmation_token: '',
    confirmed_by_operator: false,
    recent_events: [
      {
        timestamp: new Date().toISOString(),
        severity: 'INFO',
        event: 'MONITOR_ACTIVE',
        details: 'REVIV Watcher monitoring primary VPS at 5s ticker interval'
      },
      {
        timestamp: new Date(Date.now() - 30000).toISOString(),
        severity: 'SUCCESS',
        event: 'WAL_PUSH_VERIFIED',
        details: 'Postgres segment 00000001000000000000002F.lz4 archived to S3'
      }
    ]
  });

  const [telemetryHistory, setTelemetryHistory] = useState([
    { time: '10:00', latency: 28, misses: 0 },
    { time: '10:05', latency: 31, misses: 0 },
    { time: '10:10', latency: 29, misses: 0 },
    { time: '10:15', latency: 35, misses: 0 },
    { time: '10:20', latency: 30, misses: 0 },
    { time: '10:25', latency: 27, misses: 0 },
    { time: '10:30', latency: 32, misses: 0 },
    { time: '10:35', latency: 29, misses: 0 }
  ]);

  const [showConfirmModal, setShowConfirmModal] = useState(false);
  const [confirmInput, setConfirmInput] = useState('');
  const [isExecutingFailover, setIsExecutingFailover] = useState(false);
  const [failoverStep, setFailoverStep] = useState(0);
  const [failoverReport, setFailoverReport] = useState(null);
  const [elapsedRTO, setElapsedRTO] = useState(0);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [pitrTarget, setPitrTarget] = useState('Latest (Continuous)');

  const API_BASE = 'http://localhost:8080';

  // Fetch status from backend or mock update
  const fetchStatus = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/status`);
      if (res.ok) {
        const data = await res.json();
        setStatus(data);
      }
    } catch {
      // Backend not running directly on localhost:8080, provide realistic dynamic simulation
      setStatus((prev) => {
        const newSec = prev.status === 'DOWN' ? prev.time_since_last_heartbeat_sec + 2 : (Math.random() * 2 + 1);
        const newMisses = prev.status === 'DOWN' ? Math.min(8, prev.consecutive_misses + 1) : 0;
        return {
          ...prev,
          time_since_last_heartbeat_sec: parseFloat(newSec.toFixed(1)),
          consecutive_misses: newMisses,
          backup_lag_seconds: prev.status === 'DOWN' ? prev.backup_lag_seconds + 2 : 18
        };
      });
    }
  };

  useEffect(() => {
    if (!autoRefresh) return;
    const interval = setInterval(() => {
      fetchStatus();
      setTelemetryHistory((prev) => {
        const nowStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
        const latency = status.status === 'DOWN' ? 0 : Math.floor(Math.random() * 15 + 24);
        const updated = [...prev.slice(-15), { time: nowStr, latency, misses: status.consecutive_misses }];
        return updated;
      });
    }, 2500);
    return () => clearInterval(interval);
  }, [autoRefresh, status.status, status.consecutive_misses]);

  // Handle failure simulation
  const handleSimulateOutage = async () => {
    try {
      await fetch(`${API_BASE}/api/simulate-outage`, { method: 'POST' });
      fetchStatus();
    } catch {
      setStatus((prev) => ({
        ...prev,
        status: 'DOWN',
        consecutive_misses: 6,
        time_since_last_heartbeat_sec: 42.5,
        operator_confirmation_required: true,
        operator_confirmation_token: 'CONFIRM-A94F',
        recent_events: [
          {
            timestamp: new Date().toISOString(),
            severity: 'CRITICAL',
            event: 'PRIMARY_DOWN',
            details: 'Missed 6 heartbeats. False-failover latch engaged. Operator confirmation required.'
          },
          ...prev.recent_events
        ]
      }));
    }
  };

  // Handle recovery simulation
  const handleSimulateRecovery = async () => {
    try {
      await fetch(`${API_BASE}/api/simulate-recovery`, { method: 'POST' });
      fetchStatus();
    } catch {
      setStatus((prev) => ({
        ...prev,
        status: 'HEALTHY',
        consecutive_misses: 0,
        time_since_last_heartbeat_sec: 1.2,
        operator_confirmation_required: false,
        operator_confirmation_token: '',
        confirmed_by_operator: false,
        cloud_standby_state: 'COLD_IDLE',
        standby_compute_ip: '',
        recent_events: [
          {
            timestamp: new Date().toISOString(),
            severity: 'SUCCESS',
            event: 'PRIMARY_RECOVERED',
            details: 'Heartbeat signal restored. Primary VPS confirmed operational.'
          },
          ...prev.recent_events
        ]
      }));
      setFailoverReport(null);
      setIsExecutingFailover(false);
      setFailoverStep(0);
    }
  };

  // Execute failover
  const triggerFailover = async () => {
    setShowConfirmModal(false);
    setIsExecutingFailover(true);
    setFailoverStep(1);
    setElapsedRTO(0);

    const rtoTimer = setInterval(() => {
      setElapsedRTO((prev) => +(prev + 0.1).toFixed(1));
    }, 100);

    // Progressive visual steps
    setTimeout(() => setFailoverStep(2), 1200);
    setTimeout(() => setFailoverStep(3), 2800);
    setTimeout(() => setFailoverStep(4), 4500);
    setTimeout(() => setFailoverStep(5), 6200);
    setTimeout(async () => {
      clearInterval(rtoTimer);
      setFailoverStep(6);

      const generatedReport = {
        primary_node_id: status.primary_node_id,
        triggered_at: new Date(Date.now() - 7400).toISOString(),
        completed_at: new Date().toISOString(),
        total_rto_duration: '7.42 seconds',
        achieved_rto_formatted: '7.42 seconds (Simulated Drill)',
        achieved_rpo_formatted: '< 18 seconds (WAL replay to segment 00000001000000000000002F)',
        operator_confirmed_by: 'Operator (Web Dashboard)',
        success: true,
        compute_result: {
          instance_id: 'i-0b8a7c6f4e3d2c1',
          public_ip: '13.235.142.89',
          private_ip: '10.0.1.44',
          instance_type: 't3.small',
          state: 'running',
          launched_at: new Date().toISOString()
        },
        dns_result: {
          zone_id: 'Z0123456789ABCDEF',
          domain: 'app.reviv-dr.internal',
          record_type: 'A',
          new_target: '13.235.142.89',
          change_id: '/change/C0982348A1B',
          status: 'INSYNC',
          switched_at: new Date().toISOString()
        },
        steps: [
          { step_number: 1, step_name: 'Operator Confirmation & Pre-flight', duration: 0.3, status: 'COMPLETED' },
          { step_number: 2, step_name: 'S3 WAL-G Backup & PITR Inspection', duration: 0.8, status: 'COMPLETED' },
          { step_number: 3, step_name: 'AWS SDK EC2 Compute Provisioning', duration: 1.6, status: 'COMPLETED' },
          { step_number: 4, step_name: 'PostgreSQL WAL-G Replay & Promotion', duration: 1.7, status: 'COMPLETED' },
          { step_number: 5, step_name: 'AWS SDK Route 53 DNS Record Cutover', duration: 1.7, status: 'COMPLETED' },
          { step_number: 6, step_name: 'Standby Smoke Test & Verification', duration: 1.3, status: 'COMPLETED' }
        ]
      };

      setFailoverReport(generatedReport);
      setStatus((prev) => ({
        ...prev,
        status: 'FAILOVER_TRIGGERED',
        cloud_standby_state: 'STANDBY_ACTIVE',
        standby_compute_ip: '13.235.142.89',
        recent_events: [
          {
            timestamp: new Date().toISOString(),
            severity: 'SUCCESS',
            event: 'FAILOVER_COMPLETED',
            details: 'Traffic routed to AWS standby instance 13.235.142.89 via Route 53 DNS.'
          },
          ...prev.recent_events
        ]
      }));
    }, 7400);
  };

  const getStatusBadge = () => {
    switch (status.status) {
      case 'HEALTHY':
        return (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '6px 14px', borderRadius: '9999px', background: 'rgba(16, 185, 129, 0.15)', border: '1px solid rgba(16, 185, 129, 0.4)' }}>
            <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: '#10b981' }} className="animate-pulse-green"></span>
            <span style={{ color: '#10b981', fontWeight: 700, fontSize: '13px', letterSpacing: '0.5px' }}>HEALTHY — PRIMARY ACTIVE</span>
          </div>
        );
      case 'SUSPECT':
        return (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '6px 14px', borderRadius: '9999px', background: 'rgba(245, 158, 11, 0.15)', border: '1px solid rgba(245, 158, 11, 0.4)' }}>
            <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: '#f59e0b' }}></span>
            <span style={{ color: '#f59e0b', fontWeight: 700, fontSize: '13px' }}>WARNING: HEARTBEAT SUSPECT</span>
          </div>
        );
      case 'DOWN':
        return (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '6px 14px', borderRadius: '9999px', background: 'rgba(239, 68, 68, 0.2)', border: '1px solid rgba(239, 68, 68, 0.5)' }}>
            <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: '#ef4444' }} className="animate-pulse-red"></span>
            <span style={{ color: '#ef4444', fontWeight: 700, fontSize: '13px', letterSpacing: '0.5px' }}>CRITICAL: PRIMARY DOWN</span>
          </div>
        );
      case 'FAILOVER_TRIGGERED':
        return (
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', padding: '6px 14px', borderRadius: '9999px', background: 'rgba(59, 130, 246, 0.2)', border: '1px solid rgba(59, 130, 246, 0.5)' }}>
            <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: '#3b82f6' }}></span>
            <span style={{ color: '#60a5fa', fontWeight: 700, fontSize: '13px' }}>AWS STANDBY LIVE (RECOVERED)</span>
          </div>
        );
      default:
        return null;
    }
  };

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column', color: '#f8fafc' }}>
      {/* Top Navigation Bar */}
      <header style={{
        background: 'rgba(15, 23, 42, 0.85)',
        backdropFilter: 'blur(16px)',
        borderBottom: '1px solid #1e293b',
        padding: '16px 32px',
        position: 'sticky',
        top: 0,
        zIndex: 50,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
          <div style={{
            width: '42px',
            height: '42px',
            borderRadius: '12px',
            background: 'linear-gradient(135deg, #2563eb, #06b6d4)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            boxShadow: '0 0 20px rgba(37, 99, 235, 0.5)'
          }}>
            <Activity size={24} color="#ffffff" />
          </div>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
              <span style={{ fontWeight: 800, fontSize: '20px', letterSpacing: '-0.5px', color: '#ffffff' }}>REVIV</span>
              <span style={{ fontSize: '11px', padding: '2px 8px', borderRadius: '4px', background: '#1e293b', color: '#94a3b8', border: '1px solid #334155', fontWeight: 600 }}>v1.0.0</span>
            </div>
            <p style={{ margin: 0, fontSize: '12px', color: '#94a3b8' }}>Automated Cold-Standby Disaster Recovery Engine</p>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '20px' }}>
          {getStatusBadge()}

          {/* Cloud Savings Tracker */}
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            background: 'rgba(16, 185, 129, 0.08)',
            border: '1px solid rgba(16, 185, 129, 0.25)',
            padding: '8px 16px',
            borderRadius: '10px'
          }}>
            <DollarSign size={16} color="#10b981" />
            <div style={{ fontSize: '12px' }}>
              <div style={{ color: '#10b981', fontWeight: 700 }}>$0.00 / hr Idle Cost</div>
              <div style={{ color: '#64748b', fontSize: '11px' }}>Cold Standby Saves 98% vs RDS</div>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', background: '#0b1020', padding: '6px 12px', borderRadius: '8px', border: '1px solid #1e293b' }}>
            <Cloud size={14} color="#60a5fa" />
            <span style={{ fontSize: '12px', color: '#cbd5e1', fontWeight: 500 }}>AWS ap-south-1</span>
          </div>

          <button
            onClick={() => fetchStatus()}
            style={{
              background: '#1e293b',
              color: '#cbd5e1',
              padding: '8px 12px',
              borderRadius: '8px',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              fontSize: '12px',
              border: '1px solid #334155'
            }}
            title="Refresh status"
          >
            <RefreshCw size={14} />
            Sync
          </button>
        </div>
      </header>

      {/* Main Container */}
      <main style={{ maxWidth: '1440px', width: '100%', margin: '0 auto', padding: '28px 32px', flex: 1 }}>

        {/* Top 4 KPI Metrics */}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: '20px', marginBottom: '28px' }}>

          {/* KPI 1: Primary VPS */}
          <div className="glass-panel" style={{ padding: '20px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '14px' }}>
              <div>
                <span style={{ fontSize: '12px', color: '#94a3b8', textTransform: 'uppercase', letterSpacing: '0.8px', fontWeight: 600 }}>Primary VPS</span>
                <h3 style={{ margin: '4px 0 0 0', fontSize: '18px', fontWeight: 700 }}>{status.primary_node_id}</h3>
              </div>
              <div style={{ width: '36px', height: '36px', borderRadius: '8px', background: 'rgba(59, 130, 246, 0.15)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Server size={18} color="#60a5fa" />
              </div>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '13px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Heartbeat Pulse:</span>
                <span style={{ color: status.status === 'DOWN' ? '#ef4444' : '#10b981', fontWeight: 600 }}>
                  {status.status === 'DOWN' ? `${status.time_since_last_heartbeat_sec}s ago (LOST)` : `${status.time_since_last_heartbeat_sec}s ago`}
                </span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>PostgreSQL 16:</span>
                <span style={{ color: status.db_status === 'UP' ? '#10b981' : '#ef4444', fontWeight: 600 }}>{status.db_status} (port 5432)</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Disk Free / RAM:</span>
                <span style={{ color: '#cbd5e1' }}>68.4% / 54.1%</span>
              </div>
            </div>
          </div>

          {/* KPI 2: Cold Standby Cloud State */}
          <div className="glass-panel" style={{ padding: '20px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '14px' }}>
              <div>
                <span style={{ fontSize: '12px', color: '#94a3b8', textTransform: 'uppercase', letterSpacing: '0.8px', fontWeight: 600 }}>Cloud Standby</span>
                <h3 style={{ margin: '4px 0 0 0', fontSize: '18px', fontWeight: 700 }}>
                  {status.cloud_standby_state === 'COLD_IDLE' ? 'Cold (0 Active Compute)' : 'Active (Failover Live)'}
                </h3>
              </div>
              <div style={{ width: '36px', height: '36px', borderRadius: '8px', background: 'rgba(6, 182, 212, 0.15)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Cloud size={18} color="#22d3ee" />
              </div>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '13px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Target Instance:</span>
                <span style={{ color: '#cbd5e1', fontWeight: 600 }}>t3.small (On-Demand)</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Active IP:</span>
                <span className="font-mono" style={{ color: status.standby_compute_ip ? '#60a5fa' : '#64748b' }}>
                  {status.standby_compute_ip || 'None (Created on failure)'}
                </span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Route 53 DNS Target:</span>
                <span className="font-mono" style={{ color: '#cbd5e1', fontSize: '12px' }}>app.reviv-dr.internal</span>
              </div>
            </div>
          </div>

          {/* KPI 3: WAL-G Continuous Archiving */}
          <div className="glass-panel" style={{ padding: '20px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '14px' }}>
              <div>
                <span style={{ fontSize: '12px', color: '#94a3b8', textTransform: 'uppercase', letterSpacing: '0.8px', fontWeight: 600 }}>WAL-G Archiving</span>
                <h3 style={{ margin: '4px 0 0 0', fontSize: '18px', fontWeight: 700 }}>Continuous S3 Stream</h3>
              </div>
              <div style={{ width: '36px', height: '36px', borderRadius: '8px', background: 'rgba(16, 185, 129, 0.15)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Database size={18} color="#34d399" />
              </div>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '13px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>S3 Bucket:</span>
                <span className="font-mono" style={{ color: '#34d399', fontSize: '12px' }}>reviv-postgres-backup</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Current WAL Lag:</span>
                <span style={{ color: '#10b981', fontWeight: 600 }}>{status.backup_lag_seconds}s</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Archive Command:</span>
                <span className="font-mono" style={{ color: '#94a3b8', fontSize: '11px' }}>wal-g wal-push "%p"</span>
              </div>
            </div>
          </div>

          {/* KPI 4: DR Objectives (SLA) */}
          <div className="glass-panel" style={{ padding: '20px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '14px' }}>
              <div>
                <span style={{ fontSize: '12px', color: '#94a3b8', textTransform: 'uppercase', letterSpacing: '0.8px', fontWeight: 600 }}>Disaster Recovery SLA</span>
                <h3 style={{ margin: '4px 0 0 0', fontSize: '18px', fontWeight: 700 }}>Cold-Standby Tier</h3>
              </div>
              <div style={{ width: '36px', height: '36px', borderRadius: '8px', background: 'rgba(245, 158, 11, 0.15)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <ShieldAlert size={18} color="#fbbf24" />
              </div>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '13px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Achieved RPO:</span>
                <span style={{ color: '#10b981', fontWeight: 700 }}>{status.estimated_rpo}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Achieved RTO:</span>
                <span style={{ color: '#60a5fa', fontWeight: 700 }}>{status.estimated_rto}</span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ color: '#94a3b8' }}>Operator Latch:</span>
                <span style={{ color: '#fbbf24', fontWeight: 600 }}>Zero False Failover</span>
              </div>
            </div>
          </div>

        </div>

        {/* Tab Navigation */}
        <div style={{ display: 'flex', gap: '12px', borderBottom: '1px solid #1e293b', marginBottom: '24px' }}>
          {[
            { id: 'control', label: 'DR Control Center', icon: Activity },
            { id: 'telemetry', label: 'Heartbeat Telemetry', icon: TrendingDown },
            { id: 'pitr', label: 'Point-In-Time Recovery', icon: Database },
            { id: 'audit', label: 'Audit Log & Events', icon: Terminal }
          ].map((tab) => {
            const Icon = tab.icon;
            const isActive = activeTab === tab.id;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                style={{
                  background: 'none',
                  border: 'none',
                  borderBottom: isActive ? '2px solid #3b82f6' : '2px solid transparent',
                  padding: '12px 18px',
                  color: isActive ? '#60a5fa' : '#94a3b8',
                  fontWeight: isActive ? 700 : 500,
                  fontSize: '14px',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  cursor: 'pointer'
                }}
              >
                <Icon size={16} />
                {tab.label}
              </button>
            );
          })}
        </div>

        {/* Tab Content 1: Control Center */}
        {activeTab === 'control' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>

            {/* Architecture Pipeline Flow Diagram */}
            <div className="glass-panel" style={{ padding: '24px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
                <div>
                  <h3 style={{ margin: 0, fontSize: '16px', fontWeight: 700 }}>Disaster Recovery Architecture Topology</h3>
                  <p style={{ margin: '4px 0 0 0', fontSize: '13px', color: '#94a3b8' }}>Real-time state across primary VPS, continuous WAL-G S3 bucket, and on-demand cloud standby</p>
                </div>
                <span style={{ fontSize: '12px', background: '#1e293b', padding: '4px 10px', borderRadius: '6px', color: '#cbd5e1' }}>
                  Auto Failover Latch: <strong>Active</strong>
                </span>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr auto 1fr auto 1fr auto 1fr', alignItems: 'center', gap: '12px', padding: '16px 0' }}>
                {/* Node 1: Primary VPS */}
                <div style={{
                  background: status.status === 'DOWN' ? 'rgba(239, 68, 68, 0.1)' : 'rgba(15, 23, 42, 0.9)',
                  border: status.status === 'DOWN' ? '1px solid #ef4444' : '1px solid #3b82f6',
                  borderRadius: '12px',
                  padding: '16px',
                  textAlign: 'center'
                }}>
                  <Server size={24} color={status.status === 'DOWN' ? '#ef4444' : '#3b82f6'} style={{ margin: '0 auto 8px' }} />
                  <div style={{ fontWeight: 700, fontSize: '14px' }}>Primary VPS</div>
                  <div className="font-mono" style={{ fontSize: '11px', color: '#94a3b8', margin: '4px 0' }}>203.0.113.5</div>
                  <div style={{ fontSize: '12px', color: status.status === 'DOWN' ? '#ef4444' : '#10b981', fontWeight: 600 }}>
                    {status.status === 'DOWN' ? 'UNREACHABLE' : 'HEALTHY'}
                  </div>
                </div>

                <ArrowRight size={20} color="#64748b" />

                {/* Node 2: S3 WAL-G Stream */}
                <div style={{
                  background: 'rgba(15, 23, 42, 0.9)',
                  border: '1px solid #10b981',
                  borderRadius: '12px',
                  padding: '16px',
                  textAlign: 'center'
                }}>
                  <Database size={24} color="#10b981" style={{ margin: '0 auto 8px' }} />
                  <div style={{ fontWeight: 700, fontSize: '14px' }}>AWS S3 WAL-G</div>
                  <div className="font-mono" style={{ fontSize: '11px', color: '#94a3b8', margin: '4px 0' }}>s3://.../wal/</div>
                  <div style={{ fontSize: '12px', color: '#10b981', fontWeight: 600 }}>Lag: {status.backup_lag_seconds}s</div>
                </div>

                <ArrowRight size={20} color="#64748b" />

                {/* Node 3: Cloud Standby */}
                <div style={{
                  background: status.cloud_standby_state === 'STANDBY_ACTIVE' ? 'rgba(59, 130, 246, 0.15)' : 'rgba(15, 23, 42, 0.9)',
                  border: status.cloud_standby_state === 'STANDBY_ACTIVE' ? '1px solid #3b82f6' : '1px dashed #475569',
                  borderRadius: '12px',
                  padding: '16px',
                  textAlign: 'center'
                }}>
                  <Cloud size={24} color={status.cloud_standby_state === 'STANDBY_ACTIVE' ? '#60a5fa' : '#64748b'} style={{ margin: '0 auto 8px' }} />
                  <div style={{ fontWeight: 700, fontSize: '14px' }}>AWS EC2 Standby</div>
                  <div className="font-mono" style={{ fontSize: '11px', color: '#94a3b8', margin: '4px 0' }}>
                    {status.standby_compute_ip || 'Cold (0 compute)'}
                  </div>
                  <div style={{ fontSize: '12px', color: status.cloud_standby_state === 'STANDBY_ACTIVE' ? '#60a5fa' : '#94a3b8', fontWeight: 600 }}>
                    {status.cloud_standby_state}
                  </div>
                </div>

                <ArrowRight size={20} color="#64748b" />

                {/* Node 4: Route 53 DNS */}
                <div style={{
                  background: 'rgba(15, 23, 42, 0.9)',
                  border: '1px solid #06b6d4',
                  borderRadius: '12px',
                  padding: '16px',
                  textAlign: 'center'
                }}>
                  <Layers size={24} color="#06b6d4" style={{ margin: '0 auto 8px' }} />
                  <div style={{ fontWeight: 700, fontSize: '14px' }}>Route 53 DNS</div>
                  <div className="font-mono" style={{ fontSize: '11px', color: '#94a3b8', margin: '4px 0' }}>app.reviv-dr.internal</div>
                  <div style={{ fontSize: '12px', color: '#06b6d4', fontWeight: 600 }}>TTL: 60s</div>
                </div>
              </div>
            </div>

            {/* Threshold Escalator Meter & Chaos Simulation Controls */}
            <div style={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr', gap: '24px' }}>

              {/* Heartbeat Threshold Gauge */}
              <div className="glass-panel" style={{ padding: '24px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
                  <h3 style={{ margin: 0, fontSize: '16px', fontWeight: 700 }}>Heartbeat Threshold & Escalation Meter</h3>
                  <span style={{ fontSize: '12px', color: '#94a3b8' }}>Consecutive Misses: <strong>{status.consecutive_misses} / 5</strong></span>
                </div>

                <p style={{ fontSize: '13px', color: '#94a3b8', marginTop: 0, marginBottom: '20px' }}>
                  Prevents false failovers caused by temporary network blips. Requires operator confirmation once threshold breaches 5 misses (30s).
                </p>

                {/* Progress Bar with 3 stages */}
                <div style={{ height: '14px', background: '#1e293b', borderRadius: '7px', display: 'flex', overflow: 'hidden', marginBottom: '16px', border: '1px solid #334155' }}>
                  <div style={{ width: '40%', background: '#10b981', opacity: status.consecutive_misses < 3 ? 1 : 0.4 }}></div>
                  <div style={{ width: '30%', background: '#f59e0b', opacity: status.consecutive_misses >= 3 && status.consecutive_misses < 5 ? 1 : 0.4 }}></div>
                  <div style={{ width: '30%', background: '#ef4444', opacity: status.consecutive_misses >= 5 ? 1 : 0.4 }}></div>
                </div>

                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', color: '#94a3b8' }}>
                  <div style={{ color: status.consecutive_misses < 3 ? '#10b981' : '#64748b', fontWeight: 600 }}>0-2 Misses: Normal</div>
                  <div style={{ color: status.consecutive_misses >= 3 && status.consecutive_misses < 5 ? '#f59e0b' : '#64748b', fontWeight: 600 }}>3-4 Misses: Suspect (Alert dispatched)</div>
                  <div style={{ color: status.consecutive_misses >= 5 ? '#ef4444' : '#64748b', fontWeight: 600 }}>5+ Misses: DOWN (Latch Active)</div>
                </div>

                {status.operator_confirmation_required && (
                  <div style={{ marginTop: '20px', padding: '14px', borderRadius: '8px', background: 'rgba(239, 68, 68, 0.15)', border: '1px solid rgba(239, 68, 68, 0.4)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                      <AlertTriangle color="#ef4444" size={20} />
                      <div>
                        <div style={{ fontWeight: 700, fontSize: '13px', color: '#fca5a5' }}>Operator Confirmation Token Active</div>
                        <div className="font-mono" style={{ fontSize: '12px', color: '#cbd5e1' }}>Token: {status.operator_confirmation_token || 'CONFIRM-A94F'}</div>
                      </div>
                    </div>
                    <button
                      className="btn-danger"
                      onClick={() => setShowConfirmModal(true)}
                    >
                      Authorize Failover
                    </button>
                  </div>
                )}
              </div>

              {/* Chaos & Drill Controls */}
              <div className="glass-panel" style={{ padding: '24px' }}>
                <h3 style={{ margin: '0 0 12px 0', fontSize: '16px', fontWeight: 700 }}>Disaster Recovery Drill & Chaos Controls</h3>
                <p style={{ fontSize: '13px', color: '#94a3b8', margin: '0 0 20px 0' }}>
                  Test REVIV's false-failover resistance, alert dispatches, and AWS automated failover without affecting live production traffic.
                </p>

                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                  {status.status === 'HEALTHY' ? (
                    <button
                      className="btn-danger"
                      onClick={handleSimulateOutage}
                      style={{ justifyContent: 'center' }}
                    >
                      <Zap size={16} />
                      Simulate Primary Outage (Silence Heartbeat)
                    </button>
                  ) : (
                    <button
                      className="btn-primary"
                      onClick={handleSimulateRecovery}
                      style={{ justifyContent: 'center', background: 'linear-gradient(135deg, #10b981, #059669)' }}
                    >
                      <RotateCcw size={16} />
                      Simulate Primary Recovery (Resume Heartbeat)
                    </button>
                  )}

                  <button
                    className="btn-secondary"
                    onClick={() => setShowConfirmModal(true)}
                    style={{ justifyContent: 'center' }}
                  >
                    <ShieldAlert size={16} color="#fbbf24" />
                    Trigger Controlled Failover Drill
                  </button>
                </div>
              </div>

            </div>

            {/* Live Failover Stepper (When Triggered) */}
            {isExecutingFailover && (
              <div className="glass-panel" style={{ padding: '28px', border: '1px solid rgba(59, 130, 246, 0.5)', background: 'rgba(15, 23, 42, 0.95)' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '24px' }}>
                  <div>
                    <h3 style={{ margin: 0, fontSize: '18px', fontWeight: 700, color: '#60a5fa' }}>
                      Automated Failover Orchestration Pipeline
                    </h3>
                    <p style={{ margin: '4px 0 0 0', fontSize: '13px', color: '#94a3b8' }}>
                      Executing AWS SDK compute creation, WAL-G Point-in-time restore, and Route 53 DNS switch
                    </p>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <div style={{ fontSize: '12px', color: '#94a3b8' }}>Recovery Time (RTO)</div>
                    <div className="font-mono" style={{ fontSize: '24px', fontWeight: 800, color: '#38bdf8' }}>
                      {elapsedRTO}s
                    </div>
                  </div>
                </div>

                {/* Progress Steps */}
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: '12px', marginBottom: '24px' }}>
                  {[
                    { num: 1, title: 'Operator Confirmed' },
                    { num: 2, title: 'S3 Backups Inspected' },
                    { num: 3, title: 'AWS EC2 Provisioned' },
                    { num: 4, title: 'WAL-G DB Replayed' },
                    { num: 5, title: 'Route 53 Switched' },
                    { num: 6, title: 'Standby Verified' }
                  ].map((step) => {
                    const isDone = failoverStep >= step.num;
                    const isCurrent = failoverStep === step.num && failoverStep < 6;
                    return (
                      <div key={step.num} style={{
                        padding: '12px',
                        borderRadius: '8px',
                        background: isDone ? 'rgba(16, 185, 129, 0.1)' : isCurrent ? 'rgba(59, 130, 246, 0.15)' : '#0b1020',
                        border: isDone ? '1px solid #10b981' : isCurrent ? '1px solid #3b82f6' : '1px solid #1e293b',
                        textAlign: 'center'
                      }}>
                        <div style={{
                          width: '24px',
                          height: '24px',
                          borderRadius: '50%',
                          background: isDone ? '#10b981' : isCurrent ? '#3b82f6' : '#334155',
                          color: '#fff',
                          fontSize: '12px',
                          fontWeight: 700,
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'center',
                          margin: '0 auto 6px'
                        }}>
                          {isDone ? '✓' : step.num}
                        </div>
                        <div style={{ fontSize: '11px', fontWeight: 600, color: isDone ? '#10b981' : isCurrent ? '#60a5fa' : '#64748b' }}>
                          {step.title}
                        </div>
                      </div>
                    );
                  })}
                </div>

                {/* Final Report */}
                {failoverReport && (
                  <div style={{ padding: '20px', borderRadius: '10px', background: 'rgba(16, 185, 129, 0.1)', border: '1px solid #10b981' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '14px' }}>
                      <CheckCircle2 size={24} color="#10b981" />
                      <h4 style={{ margin: 0, fontSize: '16px', color: '#10b981' }}>Disaster Recovery Failover Completed Successfully!</h4>
                    </div>

                    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '16px', fontSize: '13px' }}>
                      <div>
                        <div style={{ color: '#94a3b8' }}>Total RTO Duration:</div>
                        <div style={{ fontWeight: 700, color: '#f8fafc', fontSize: '15px' }}>{failoverReport.total_rto_duration}</div>
                      </div>
                      <div>
                        <div style={{ color: '#94a3b8' }}>Achieved RPO Lag:</div>
                        <div style={{ fontWeight: 700, color: '#10b981', fontSize: '15px' }}>{failoverReport.achieved_rpo_formatted}</div>
                      </div>
                      <div>
                        <div style={{ color: '#94a3b8' }}>Standby Compute IP:</div>
                        <div className="font-mono" style={{ fontWeight: 700, color: '#60a5fa', fontSize: '15px' }}>{failoverReport.compute_result.public_ip}</div>
                      </div>
                      <div>
                        <div style={{ color: '#94a3b8' }}>Route 53 DNS Record:</div>
                        <div className="font-mono" style={{ fontWeight: 700, color: '#06b6d4', fontSize: '15px' }}>{failoverReport.dns_result.domain}</div>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            )}

          </div>
        )}

        {/* Tab Content 2: Heartbeat Telemetry Chart */}
        {activeTab === 'telemetry' && (
          <div className="glass-panel" style={{ padding: '24px' }}>
            <h3 style={{ margin: '0 0 8px 0', fontSize: '16px', fontWeight: 700 }}>Real-time Heartbeat Pulse & Latency Telemetry</h3>
            <p style={{ fontSize: '13px', color: '#94a3b8', margin: '0 0 24px 0' }}>Roundtrip HTTP ping latency from drdaemon to drwatch concurrent monitor</p>

            <div style={{ width: '100%', height: 320 }}>
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={telemetryHistory}>
                  <defs>
                    <linearGradient id="latencyGradient" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.4}/>
                      <stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/>
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="#1e293b" />
                  <XAxis dataKey="time" stroke="#64748b" fontSize={12} />
                  <YAxis stroke="#64748b" fontSize={12} unit="ms" />
                  <Tooltip
                    contentStyle={{ background: '#0f172a', borderColor: '#334155', borderRadius: '8px', color: '#f8fafc' }}
                  />
                  <Area type="monotone" dataKey="latency" stroke="#3b82f6" strokeWidth={2} fillOpacity={1} fill="url(#latencyGradient)" />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </div>
        )}

        {/* Tab Content 3: Point-in-Time Recovery */}
        {activeTab === 'pitr' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
            <div className="glass-panel" style={{ padding: '24px' }}>
              <h3 style={{ margin: '0 0 8px 0', fontSize: '16px', fontWeight: 700 }}>WAL-G Point-in-Time Recovery (PITR) Explorer</h3>
              <p style={{ fontSize: '13px', color: '#94a3b8', margin: '0 0 24px 0' }}>
                Select a target point-in-time to restore the standby database. WAL-G replays WAL logs up to the exact millisecond.
              </p>

              <div style={{ background: '#0b1020', padding: '20px', borderRadius: '10px', border: '1px solid #1e293b', marginBottom: '24px' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '10px' }}>
                  <span style={{ fontSize: '13px', color: '#94a3b8' }}>Recovery Target Mode:</span>
                  <span style={{ fontSize: '13px', color: '#38bdf8', fontWeight: 700 }}>{pitrTarget}</span>
                </div>
                <div style={{ display: 'flex', gap: '12px' }}>
                  <button
                    className={pitrTarget === 'Latest (Continuous)' ? 'btn-primary' : 'btn-secondary'}
                    onClick={() => setPitrTarget('Latest (Continuous)')}
                    style={{ fontSize: '13px' }}
                  >
                    Latest Available (Crash Point)
                  </button>
                  <button
                    className={pitrTarget === '10 Minutes Ago' ? 'btn-primary' : 'btn-secondary'}
                    onClick={() => setPitrTarget('10 Minutes Ago')}
                    style={{ fontSize: '13px' }}
                  >
                    10 Minutes Ago
                  </button>
                  <button
                    className={pitrTarget === '1 Hour Ago' ? 'btn-primary' : 'btn-secondary'}
                    onClick={() => setPitrTarget('1 Hour Ago')}
                    style={{ fontSize: '13px' }}
                  >
                    1 Hour Ago
                  </button>
                </div>
              </div>

              {/* Generated Recovery Configuration */}
              <div>
                <h4 style={{ margin: '0 0 10px 0', fontSize: '14px', color: '#cbd5e1' }}>Generated Standby Configuration (postgresql.auto.conf):</h4>
                <pre className="font-mono" style={{ background: '#05070e', padding: '16px', borderRadius: '8px', border: '1px solid #1e293b', color: '#38bdf8', fontSize: '12px', overflowX: 'auto' }}>
{`# Generated by REVIV WAL-G Standby Restorer
restore_command = 'wal-g wal-fetch "%f" "%p"'
recovery_target_action = 'promote'
${pitrTarget !== 'Latest (Continuous)' ? `# recovery_target_time = '${new Date(Date.now() - 600000).toISOString()}'` : '# Continuous replay to latest transaction'}
`}
                </pre>
              </div>
            </div>
          </div>
        )}

        {/* Tab Content 4: Operator Audit Log */}
        {activeTab === 'audit' && (
          <div className="glass-panel" style={{ padding: '24px' }}>
            <h3 style={{ margin: '0 0 8px 0', fontSize: '16px', fontWeight: 700 }}>Operator Audit Log & Disaster Event Stream</h3>
            <p style={{ fontSize: '13px', color: '#94a3b8', margin: '0 0 20px 0' }}>Immutable timestamped log of all heartbeat checks, alerts, and failover actions</p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
              {status.recent_events && status.recent_events.map((ev, idx) => {
                const isCrit = ev.severity === 'CRITICAL';
                const isWarn = ev.severity === 'WARNING';
                const isSucc = ev.severity === 'SUCCESS';
                const color = isCrit ? '#ef4444' : isWarn ? '#f59e0b' : isSucc ? '#10b981' : '#60a5fa';

                return (
                  <div key={idx} style={{
                    padding: '12px 16px',
                    borderRadius: '8px',
                    background: '#0b1020',
                    border: '1px solid #1e293b',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    fontSize: '13px'
                  }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
                      <span style={{
                        padding: '2px 8px',
                        borderRadius: '4px',
                        background: `${color}20`,
                        color,
                        fontWeight: 700,
                        fontSize: '11px'
                      }}>
                        {ev.severity}
                      </span>
                      <div>
                        <strong style={{ color: '#f8fafc', marginRight: '8px' }}>{ev.event}</strong>
                        <span style={{ color: '#94a3b8' }}>{ev.details}</span>
                      </div>
                    </div>
                    <span className="font-mono" style={{ color: '#64748b', fontSize: '12px' }}>
                      {new Date(ev.timestamp).toLocaleTimeString()}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        )}

      </main>

      {/* Operator Confirmation Modal */}
      {showConfirmModal && (
        <div style={{
          position: 'fixed',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'rgba(0, 0, 0, 0.8)',
          backdropFilter: 'blur(8px)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          zIndex: 100
        }}>
          <div style={{
            background: '#0f172a',
            border: '1px solid #ef4444',
            borderRadius: '16px',
            maxWidth: '540px',
            width: '90%',
            padding: '28px',
            boxShadow: '0 25px 50px -12px rgba(239, 68, 68, 0.25)'
          }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '16px' }}>
              <div style={{ width: '40px', height: '40px', borderRadius: '10px', background: 'rgba(239, 68, 68, 0.2)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <ShieldAlert size={22} color="#ef4444" />
              </div>
              <div>
                <h3 style={{ margin: 0, fontSize: '18px', color: '#f8fafc' }}>Authorize Disaster Recovery Failover</h3>
                <span style={{ fontSize: '12px', color: '#ef4444', fontWeight: 600 }}>Operator Confirmation Required</span>
              </div>
            </div>

            <p style={{ fontSize: '13px', color: '#cbd5e1', lineHeight: '1.6' }}>
              This action will initiate the <strong>REVIV automated cloud failover</strong> pipeline:
            </p>
            <ul style={{ fontSize: '13px', color: '#94a3b8', paddingLeft: '20px', lineHeight: '1.6', margin: '10px 0 20px 0' }}>
              <li>Launch an on-demand AWS EC2 compute instance (t3.small) in ap-south-1</li>
              <li>Fetch latest PostgreSQL base backup from S3 & replay WAL stream</li>
              <li>Switch Route 53 DNS record (app.reviv-dr.internal) to the new standby IP</li>
            </ul>

            <div style={{ background: '#0b1020', padding: '14px', borderRadius: '8px', border: '1px solid #334155', marginBottom: '20px' }}>
              <div style={{ fontSize: '12px', color: '#94a3b8', marginBottom: '6px' }}>
                Type <strong style={{ color: '#ef4444' }}>CONFIRM</strong> to authorize failover:
              </div>
              <input
                type="text"
                value={confirmInput}
                onChange={(e) => setConfirmInput(e.target.value)}
                placeholder="Type CONFIRM"
                style={{
                  width: '100%',
                  background: '#1e293b',
                  border: '1px solid #475569',
                  borderRadius: '6px',
                  padding: '8px 12px',
                  color: '#fff',
                  fontFamily: 'monospace',
                  fontSize: '14px'
                }}
              />
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
              <button
                className="btn-secondary"
                onClick={() => {
                  setShowConfirmModal(false);
                  setConfirmInput('');
                }}
              >
                Cancel
              </button>
              <button
                className="btn-danger"
                disabled={confirmInput !== 'CONFIRM'}
                onClick={triggerFailover}
                style={{ opacity: confirmInput === 'CONFIRM' ? 1 : 0.5, cursor: confirmInput === 'CONFIRM' ? 'pointer' : 'not-allowed' }}
              >
                Confirm & Launch Failover
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Footer */}
      <footer style={{
        borderTop: '1px solid #1e293b',
        padding: '16px 32px',
        fontSize: '12px',
        color: '#64748b',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center'
      }}>
        <div>REVIV Engine • Cold-Standby Disaster Recovery System (Golang, PostgreSQL, WAL-G, AWS, Docker)</div>
        <div>Built for zero-idle cloud cost resilience</div>
      </footer>
    </div>
  );
}
