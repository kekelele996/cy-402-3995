import { useEffect, useState } from 'react'
import { Button, Modal, Form, Input, Select, DatePicker, message, Card, Table } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import FilterBar from '@/components/common/FilterBar'
import ConflictAlert from '@/components/common/ConflictAlert'
import { useCaseStore } from '@/stores/caseStore'
import { useUserStore } from '@/stores/userStore'
import { useClientStore } from '@/stores/clientStore'
import { createCase } from '@/api/case'
import { ErrorCode } from '@/constants/errorCodes'
import { CaseStatusOptions, CaseTypeOptions } from '@/constants/case'
import StatusBadge from '@/components/common/StatusBadge'
import type { CaseItem, ConflictCheck } from '@/types'

export default function Cases() {
  const store = useCaseStore()
  const userStore = useUserStore()
  const clientStore = useClientStore()
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [filters, setFilters] = useState<Record<string, unknown>>({})
  const [open, setOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [conflict, setConflict] = useState<ConflictCheck | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    userStore.fetchLawyers()
    clientStore.fetchList({ page: 1, page_size: 200 })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    store.fetchList({ page, page_size: pageSize, ...filters })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, pageSize, filters])

  const lawyerOptions = userStore.lawyers.map((l) => ({ label: l.real_name || l.username, value: l.id }))

  async function onCreate() {
    const values = await form.validateFields()
    setSubmitting(true)
    setConflict(null)
    try {
      const res: any = await createCase({
        title: values.title,
        case_type: values.case_type,
        client_id: values.client_id,
        lead_lawyer_id: values.lead_lawyer_id,
        co_lawyer_ids: values.co_lawyer_ids,
        accept_date: values.accept_date ? values.accept_date.format('YYYY-MM-DD') : undefined,
        summary: values.summary,
        opponent_name: values.opponent_name?.trim(),
        opponent_id_number: values.opponent_id_number?.trim(),
      })
      // 对方证件号缺失时后端只提示不阻断：warning 放在响应 message 中
      if (res?.data?.warning) {
        message.warning(res.data.warning)
      } else {
        message.success('案件创建成功')
      }
      setOpen(false)
      form.resetFields()
      setPage(1)
      store.fetchList({ page: 1, page_size: pageSize, ...filters })
    } catch (e: any) {
      // 利益冲突：整次拒绝，弹窗保留，内联展示冲突原因与冲突案号，调整后可重试
      const data = e?.response?.data
      if (data?.code === ErrorCode.LAWYER_CONFLICT && data?.data) {
        setConflict(data.data as ConflictCheck)
      }
      // 其他错误已由 request 拦截器统一 message.error 提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Card>
      <FilterBar
        typeOptions={CaseTypeOptions}
        statusOptions={CaseStatusOptions}
        lawyerOptions={userStore.lawyers.map((l) => ({ label: l.real_name || l.username, value: l.id }))}
        onSearch={(v) => { setFilters(v); setPage(1) }}
      />
      <Button type="primary" icon={<PlusOutlined />} style={{ marginBottom: 16 }} onClick={() => { setConflict(null); setOpen(true) }}>
        创建案件
      </Button>
      <Table<CaseItem>
        rowKey="id"
        dataSource={store.list}
        loading={false}
        pagination={{ current: page, pageSize, total: store.total, showTotal: (t) => `共 ${t} 条`, onChange: (p, ps) => { setPage(p); setPageSize(ps) } }}
        onRow={(row) => ({ onClick: () => navigate(`/cases/${row.id}`), style: { cursor: 'pointer' } })}
        columns={[
          { title: '案号', dataIndex: 'case_no' },
          { title: '标题', dataIndex: 'title' },
          { title: '类型', dataIndex: 'case_type', render: (v) => CaseTypeOptions.find((o) => o.value === v)?.label || v },
          { title: '状态', dataIndex: 'status', render: (v) => <StatusBadge status={v} /> },
          { title: '对方当事人', key: 'opponent', render: (_, r) => r.opponent_name || <span style={{ color: '#bbb' }}>未登记</span> },
          { title: '主办律师', dataIndex: 'lead_lawyer_id', render: (v) => userStore.lawyers.find((l) => l.id === v)?.real_name || v },
        ]}
      />
      <Modal
        title="创建案件"
        open={open}
        confirmLoading={submitting}
        onOk={onCreate}
        onCancel={() => setOpen(false)}
        okText="创建并预检冲突"
        width={620}
      >
        <Form form={form} layout="vertical" onValuesChange={() => conflict && setConflict(null)}>
          <Form.Item name="title" label="案件标题" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="case_type" label="案件类型" rules={[{ required: true }]}>
            <Select options={CaseTypeOptions} />
          </Form.Item>
          <Form.Item name="client_id" label="我方客户" rules={[{ required: true }]}>
            <Select options={clientStore.list.map((c) => ({ label: c.name, value: c.id }))} showSearch optionFilterProp="label" />
          </Form.Item>
          <Form.Item name="lead_lawyer_id" label="主办律师" rules={[{ required: true }]}>
            <Select options={lawyerOptions} />
          </Form.Item>
          <Form.Item name="co_lawyer_ids" label="协作律师（参与利益冲突预检）">
            <Select mode="multiple" allowClear options={lawyerOptions} placeholder="可多选" />
          </Form.Item>
          <Form.Item name="opponent_name" label="对方当事人姓名/名称">
            <Input maxLength={100} placeholder="资料缺失可留空，仅提示不阻断创建" />
          </Form.Item>
          <Form.Item name="opponent_id_number" label="对方证件号">
            <Input maxLength={50} placeholder="用于律师利益冲突预检；缺失时只提示不阻断" />
          </Form.Item>
          <Form.Item name="accept_date" label="受理日期">
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="summary" label="摘要">
            <Input.TextArea rows={2} />
          </Form.Item>
          {conflict && <ConflictAlert check={conflict} />}
        </Form>
      </Modal>
    </Card>
  )
}
