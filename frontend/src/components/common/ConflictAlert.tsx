import { Alert, Table, Tag } from 'antd'
import type { ConflictCase } from '@/types'

interface ConflictAlertProps {
  reason?: string
  cases?: ConflictCase[]
  // 操作被拒绝后返回的错误明细（data 字段即 ConflictCheck）
  check?: { reason?: string; cases?: ConflictCase[] } | null
  type?: 'error' | 'warning'
  message?: string
  style?: React.CSSProperties
}

// ConflictAlert 展示律师利益冲突预检结果：冲突原因 + 冲突案号明细表。
// 用于分配/创建被拒后的内联提示，以及案件详情中的常驻冲突提示。
export default function ConflictAlert({ reason, cases, check, type = 'error', message, style }: ConflictAlertProps) {
  const finalReason = reason || check?.reason || ''
  const finalCases = cases || check?.cases || []
  if (type === 'warning') {
    return <Alert style={style} type="warning" showIcon message={message || '资料缺失'} description={finalReason || '对方当事人证件号缺失，未执行利益冲突预检。'} />
  }
  return (
    <Alert
      style={style}
      type="error"
      showIcon
      message={message || '律师利益冲突，本次操作已拒绝（原分配保持不变，调整后可重试）'}
      description={
        <>
          {finalReason && <p style={{ marginBottom: 8 }}>{finalReason}</p>}
          {finalCases.length > 0 && (
            <Table<ConflictCase>
              size="small"
              rowKey={(r) => `${r.lawyer_id}-${r.case_id}`}
              pagination={false}
              dataSource={finalCases}
              columns={[
                { title: '冲突律师', dataIndex: 'lawyer_name', width: 100 },
                {
                  title: '冲突案号',
                  dataIndex: 'case_no',
                  render: (v: string) => <Tag color="error">{v}</Tag>,
                },
                { title: '案件标题', dataIndex: 'case_title' },
                { title: '对方姓名', dataIndex: 'opponent_name' },
                { title: '对方证件号', dataIndex: 'opponent_id_number' },
              ]}
            />
          )}
        </>
      }
    />
  )
}
