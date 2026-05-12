import { useEffect, useRef, useState } from 'react';

interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
  data?: Record<string, any>;
}

interface FlowLogViewerProps {
  logs: LogEntry[];
  title?: string;
  maxHeight?: string;
  autoScroll?: boolean;
}

const levelColors: Record<string, { bg: string; text: string; icon: string }> = {
  info: { bg: 'bg-blue-50', text: 'text-blue-700', icon: 'ℹ️' },
  debug: { bg: 'bg-gray-50', text: 'text-gray-600', icon: '🔍' },
  warn: { bg: 'bg-yellow-50', text: 'text-yellow-700', icon: '⚠️' },
  error: { bg: 'bg-red-50', text: 'text-red-700', icon: '❌' },
  success: { bg: 'bg-green-50', text: 'text-green-700', icon: '✅' },
};

export function FlowLogViewer({ 
  logs, 
  title = 'Flow Logs',
  maxHeight = '400px',
  autoScroll = true 
}: FlowLogViewerProps) {
  const [expandedEntries, setExpandedEntries] = useState<Set<number>>(new Set());
  const logEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (autoScroll && logEndRef.current) {
      logEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs, autoScroll]);

  const toggleEntry = (index: number) => {
    setExpandedEntries(prev => {
      const newSet = new Set(prev);
      if (newSet.has(index)) {
        newSet.delete(index);
      } else {
        newSet.add(index);
      }
      return newSet;
    });
  };

  const formatTimestamp = (timestamp: string) => {
    try {
      const date = new Date(timestamp);
      return date.toLocaleTimeString('en-US', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      });
    } catch {
      return timestamp;
    }
  };

  const formatData = (data: Record<string, any>): string => {
    try {
      return JSON.stringify(data, null, 2);
    } catch {
      return String(data);
    }
  };

  if (!logs || logs.length === 0) {
    return (
      <div className="border border-gray-200 rounded-lg overflow-hidden">
        <div className="bg-gray-50 px-4 py-2 border-b border-gray-200">
          <h3 className="text-sm font-semibold text-gray-700">{title}</h3>
        </div>
        <div className="p-4 text-center text-gray-500 text-sm">
          No logs available
        </div>
      </div>
    );
  }

  return (
    <div className="border border-gray-200 rounded-lg overflow-hidden">
      {/* Header */}
      <div className="bg-gray-50 px-4 py-2 border-b border-gray-200 flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-700">{title}</h3>
        <span className="text-xs text-gray-500 bg-white px-2 py-1 rounded-full">
          {logs.length} entries
        </span>
      </div>

      {/* Log entries */}
      <div 
        className="overflow-y-auto font-mono text-xs"
        style={{ maxHeight }}
      >
        {logs.map((entry, index) => {
          const levelConfig = levelColors[entry.level] || levelColors.info;
          const hasData = entry.data && Object.keys(entry.data).length > 0;
          const isExpanded = expandedEntries.has(index);

          return (
            <div
              key={index}
              className={`border-b border-gray-100 last:border-b-0 ${levelConfig.bg}`}
            >
              <div
                className="px-3 py-2 cursor-pointer hover:opacity-80 transition-opacity"
                onClick={() => hasData && toggleEntry(index)}
              >
                <div className="flex items-start gap-2">
                  <span className="text-gray-400 whitespace-nowrap select-none">
                    {formatTimestamp(entry.timestamp)}
                  </span>
                  <span className={`${levelConfig.text} font-medium select-none`}>
                    {levelConfig.icon}
                  </span>
                  <span className={`${levelConfig.text} flex-1 break-all`}>
                    {entry.message}
                  </span>
                  {hasData && (
                    <span className="text-gray-400 select-none">
                      {isExpanded ? '▼' : '▶'}
                    </span>
                  )}
                </div>
              </div>

              {/* Expanded data */}
              {hasData && isExpanded && (
                <div className="px-3 pb-2 pl-8">
                  <pre className="bg-white p-2 rounded border border-gray-200 overflow-x-auto text-[10px] leading-relaxed">
                    {formatData(entry.data || {})}
                  </pre>
                </div>
              )}
            </div>
          );
        })}
        <div ref={logEndRef} />
      </div>
    </div>
  );
}
