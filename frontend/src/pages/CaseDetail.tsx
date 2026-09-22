import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Card, Descriptions, Tabs, Button, Select, Space, message, Tag, Modal, Form, Input } from 'antd'
import { getCase, changeCaseStatus, assignLawyer, updateCase } from '@/api/case'
import { getClient } from '@/api/client'
import DocumentList from '@/components/common/DocumentList'
import BillingCard from '@/components/common/BillingCard'
import StatusBadge from '@/components/common/StatusBadge'
import PermissionGuard from '@/components/common/PermissionGuard'
import TimelineItem from '@/components/common/TimelineItem'
import ConflictAlert from '@/components/common/ConflictAlert'
import { useDocumentStore } from '@/stores/documentStore'
import { useBillingStore } from '@/stores/billingStore'
import { useUserStore } from '@/stores/userStore'
import { CaseStatusOptions, CaseTypeOptions } from '@/constants/case'
import { ErrorCode } from '@/constants/errorCodes'
import type { CaseItem, Client, ConflictCheck } from '@/types'

export default function CaseDetail() {
  const { id } = useParams()
  const caseId = Number(id)
  const [item, setItem] = useState<CaseItem | null>(null)
  const [conflicts, setConflicts] = useState<ConflictCheck | null>(null)
  const [warning, setWarning] = useState('')
  const [client, setClient] = useState<Client | null>(null)
  const [status, setStatus] = useState('')
  const [lawyer, setLawyer] = useState<number>()
  const [coLawyers, setCoLawyers] = useState<number[]>([])
  const [assignConflict, setAssignConflict] = useState<ConflictCheck | null>(null)
  const [assigning, setAssigning] = useState(false)
  const [editOpen, setEditOpen] = useState(false)
  const [editConflict, setEditConflict] = useState<ConflictCheck | null>(null)
  const [saving, setSaving] = useState(false)
  const [editForm] = Form.useForm()
  const docStore = useDocumentStore()
  const billingStore = useBillingStore()
  const userStore = useUserStore()

  useEffect(() => {
    userStore.fetchLawyers()
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [caseId])

  async function load() {
    const res: any = await getCase(caseId)
    const result = res.data
    setItem(result.case)
    setConflicts(result.conflicts || null)
    setWarning(result.warning || '')
    setStatus(result.case.status)
    if (result.case.client_id) {
      const cr: any = await getClient(result.case.client_id)
      setClient(cr.data.client)
    }
    docStore.fetchByCase(caseId)
    billingStore.fetchByCase(caseId)
  }

  async function onStatusChange() {
    await changeCaseStatus(caseId, status)
    message.success('状态已更新')
    load()
  }

  async function onAssign() {
    if (!lawyer) return
    setAssigning(true)
    setAssignConflict(null)
    try {
      const res: any = await assignLawyer(caseId, { lead_lawyer_id: lawyer, co_lawyer_ids: coLawyers })
      if (res?.data?.warning) {
        message.warning(res.data.warning)
      } else {
        message.success('律师已分配')
      }
      setLawyer(undefined)
      setCoLawyers([])
      load()
    } catch (e: any) {
      // 利益冲突：整次拒绝，原分配保持不变；内联展示冲突原因与冲突案号，调整后可重试
      const data = e?.response?.data
      if (data?.code === ErrorCode.LAWYER_CONFLICT && data?.data) {
        setAssignConflict(data.data as ConflictCheck)
      }
    } finally {
      setAssigning(false)
    }
  }

  function openEdit() {
    if (!item) return
    setEditConflict(null)
    editForm.setFieldsValue({
      title: item.title,
      summary: item.summary,
      opponent_name: item.opponent_name,
      opponent_id_number: item.opponent_id_number,
    })
    setEditOpen(true)
  }

  async function onSaveEdit() {
    const values = await editForm.validateFields()
    setSaving(true)
    setEditConflict(null)
    try {
      const res: any = await updateCase(caseId, {
        title: values.title?.trim(),
        summary: values.summary,
        opponent_name: values.opponent_name?.trim(),
        opponent_id_number: values.opponent_id_number?.trim(),
      })
      if (res?.data?.warning) {
        message.warning(res.data.warning)
      } else {
        message.success('案件信息已更新')
      }
      setEditOpen(false)
      load()
    } catch (e: any) {
      const data = e?.response?.data
      if (data?.code === ErrorCode.LAWYER_CONFLICT && data?.data) {
        setEditConflict(data.data as ConflictCheck)
      }
    } finally {
      setSaving(false)
    }
  }

  if (!item) return null

  const lawyerOptions = userStore.lawyers.map((l) => ({ label: l.real_name || l.username, value: l.id }))
  const lawyerName = (uid: number) => userStore.lawyers.find((l) => l.id === uid)?.real_name || `#${uid}`

  return (
    <Card>
      <Space style={{ marginBottom: 16 }}>
        <h2 style={{ margin: 0 }}>{item.case_no} {item.title}</h2>
        <StatusBadge status={item.status} />
        <Tag>{CaseTypeOptions.find((o) => o.value === item.case_type)?.label || item.case_type}</Tag>
      </Space>
      <Tabs
        items={[
          {
            key: 'info',
            label: '案件信息',
            children: (
              <>
                {conflicts?.has_conflict && (
                  <ConflictAlert
                    style={{ marginBottom: 16 }}
                    message="当前律师分配存在利益冲突，请调整主办/协作律师或核对对方当事人信息"
                    check={conflicts}
                  />
                )}
                {!conflicts?.has_conflict && warning && (
                  <ConflictAlert style={{ marginBottom: 16 }} type="warning" reason={warning} />
                )}
                <Descriptions bordered column={2} size="small">
                  <Descriptions.Item label="案号">{item.case_no}</Descriptions.Item>
                  <Descriptions.Item label="状态">{item.status}</Descriptions.Item>
                  <Descriptions.Item label="类型">{item.case_type}</Descriptions.Item>
                  <Descriptions.Item label="主办律师">{lawyerName(item.lead_lawyer_id)}</Descriptions.Item>
                  <Descriptions.Item label="协作律师">
                    {item.co_lawyer_ids?.length ? item.co_lawyer_ids.map((uid) => <Tag key={uid}>{lawyerName(uid)}</Tag>) : '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="受理日期">{item.accept_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="对方当事人姓名/名称">
                    {item.opponent_name || <span style={{ color: '#bbb' }}>未登记</span>}
                  </Descriptions.Item>
                  <Descriptions.Item label="对方证件号">
                    {item.opponent_id_number || <span style={{ color: '#bbb' }}>未登记（冲突预检未执行）</span>}
                  </Descriptions.Item>
                  <Descriptions.Item label="结案日期">{item.close_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="摘要" span={2}>{item.summary || '-'}</Descriptions.Item>
                </Descriptions>
                <PermissionGuard roles={['admin', 'lawyer']}>
                  <Space style={{ marginTop: 16 }}>
                    <Select value={status} style={{ width: 150 }} options={CaseStatusOptions} onChange={setStatus} />
                    <Button type="primary" onClick={onStatusChange}>更新状态</Button>
                    <Button onClick={openEdit}>编辑案件/对方信息</Button>
                  </Space>
                  <Space style={{ marginTop: 8 }} wrap>
                    <Select
                      placeholder="分配主办律师"
                      style={{ width: 160 }}
                      value={lawyer}
                      onChange={setLawyer}
                      options={lawyerOptions}
                    />
                    <Select
                      mode="multiple"
                      allowClear
                      placeholder="协作律师（可多选）"
                      style={{ minWidth: 220 }}
                      value={coLawyers}
                      onChange={setCoLawyers}
                      options={lawyerOptions}
                    />
                    <Button onClick={onAssign} loading={assigning} disabled={!lawyer}>分配（自动冲突预检）</Button>
                  </Space>
                  {assignConflict && (
                    <ConflictAlert style={{ marginTop: 12 }} check={assignConflict} />
                  )}
                </PermissionGuard>
              </>
            ),
          },
          {
            key: 'client',
            label: '关联客户',
            children: client ? (
              <Descriptions bordered column={1} size="small">
                <Descriptions.Item label="姓名">{client.name}</Descriptions.Item>
                <Descriptions.Item label="证件号">{client.id_number}</Descriptions.Item>
                <Descriptions.Item label="联系方式">{client.contact}</Descriptions.Item>
                <Descriptions.Item label="地址">{client.address}</Descriptions.Item>
              </Descriptions>
            ) : null,
          },
          {
            key: 'docs',
            label: '文档',
            children: <DocumentList documents={docStore.byCase} />,
          },
          {
            key: 'billings',
            label: '账单',
            children: billingStore.byCase.map((b) => <BillingCard key={b.id} item={b} />),
          },
          {
            key: 'timeline',
            label: '时间线',
            children: (
              <TimelineItem
                items={[
                  { id: 1, time: item.created_at, text: `案件创建（${item.case_no}）` },
                  { id: 2, time: item.accept_date || item.created_at, text: '案件受理' },
                  { id: 3, time: item.close_date || item.created_at, text: item.close_date ? '案件结案' : '案件进行中' },
                ]}
              />
            ),
          },
        ]}
      />
      <Modal
        title="编辑案件与对方当事人信息"
        open={editOpen}
        confirmLoading={saving}
        onOk={onSaveEdit}
        onCancel={() => setEditOpen(false)}
        okText="保存"
        width={620}
      >
        <Form form={editForm} layout="vertical" onValuesChange={() => editConflict && setEditConflict(null)}>
          <Form.Item name="title" label="案件标题" rules={[{ required: true }]}>
            <Input maxLength={200} />
          </Form.Item>
          <Form.Item name="opponent_name" label="对方当事人姓名/名称">
            <Input maxLength={100} placeholder="资料缺失可留空，仅提示不阻断保存" />
          </Form.Item>
          <Form.Item name="opponent_id_number" label="对方证件号">
            <Input maxLength={50} placeholder="用于律师利益冲突预检；缺失时只提示不阻断" />
          </Form.Item>
          <Form.Item name="summary" label="摘要">
            <Input.TextArea rows={3} />
          </Form.Item>
          {editConflict && <ConflictAlert check={editConflict} />}
        </Form>
      </Modal>
    </Card>
  )
}
