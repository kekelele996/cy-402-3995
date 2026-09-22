import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Card, Descriptions, Tabs, Button, Select, Space, message, Tag, Alert, Modal, Form, Input } from 'antd'
import { EditOutlined } from '@ant-design/icons'
import { getCase, changeCaseStatus, assignLawyer, updateCase } from '@/api/case'
import { getClient } from '@/api/client'
import DocumentList from '@/components/common/DocumentList'
import BillingCard from '@/components/common/BillingCard'
import StatusBadge from '@/components/common/StatusBadge'
import PermissionGuard from '@/components/common/PermissionGuard'
import TimelineItem from '@/components/common/TimelineItem'
import { useDocumentStore } from '@/stores/documentStore'
import { useBillingStore } from '@/stores/billingStore'
import { useUserStore } from '@/stores/userStore'
import { CaseStatusOptions, CaseTypeOptions } from '@/constants/case'
import type { CaseAssignConflictData, CaseItem, Client, LawyerConflictInfo } from '@/types'

export default function CaseDetail() {
  const { id } = useParams()
  const caseId = Number(id)
  const [item, setItem] = useState<CaseItem | null>(null)
  const [client, setClient] = useState<Client | null>(null)
  const [status, setStatus] = useState('')
  const [lawyer, setLawyer] = useState<number>()
  const [coLawyers, setCoLawyers] = useState<number[]>([])
  const [conflict, setConflict] = useState<CaseAssignConflictData | null>(null)
  const [assignWarning, setAssignWarning] = useState('')
  const [editOpen, setEditOpen] = useState(false)
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
    setItem(res.data)
    setStatus(res.data.status)
    setLawyer(res.data.lead_lawyer_id)
    setCoLawyers(Array.isArray(res.data.co_lawyer_ids) ? res.data.co_lawyer_ids : [])
    setConflict(null)
    setAssignWarning('')
    if (res.data.client_id) {
      const cr: any = await getClient(res.data.client_id)
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
    if (!lawyer || !item) return
    setConflict(null)
    setAssignWarning('')
    try {
      const res: any = await assignLawyer(caseId, { lead_lawyer_id: lawyer, co_lawyer_ids: coLawyers })
      message.success('律师已分配')
      await load()
      // load() 会清空提示，需在其完成后再写入，保证页面上持久展示
      if (res.data?.warning) {
        setAssignWarning(res.data.warning)
        message.warning(res.data.warning)
      }
    } catch (e: any) {
      const data = e?.response?.data
      if (data?.code === 40903 && data?.data) {
        // 整次拒绝、原分配保持不变：选择框回退到当前已保存的主办/协作律师
        setLawyer(item.lead_lawyer_id)
        setCoLawyers(Array.isArray(item.co_lawyer_ids) ? item.co_lawyer_ids : [])
        setConflict(data.data as CaseAssignConflictData)
      }
    }
  }

  async function onEditOpponent() {
    const values = await editForm.validateFields()
    await updateCase(caseId, {
      opponent_name: values.opponent_name,
      opponent_id_number: values.opponent_id_number,
    })
    message.success('对方当事人信息已更新，可重新执行律师分配')
    setEditOpen(false)
    load()
  }

  function openEditOpponent() {
    editForm.setFieldsValue({
      opponent_name: item?.opponent_name || '',
      opponent_id_number: item?.opponent_id_number || '',
    })
    setEditOpen(true)
  }

  if (!item) return null

  const lawyerLabel = (id: number | undefined) =>
    userStore.lawyers.find((l) => l.id === id)?.real_name || (id ? `#${id}` : '-')

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
                <Descriptions bordered column={2} size="small">
                  <Descriptions.Item label="案号">{item.case_no}</Descriptions.Item>
                  <Descriptions.Item label="状态"><StatusBadge status={item.status} /></Descriptions.Item>
                  <Descriptions.Item label="类型">{item.case_type}</Descriptions.Item>
                  <Descriptions.Item label="主办律师">{lawyerLabel(item.lead_lawyer_id)}</Descriptions.Item>
                  <Descriptions.Item label="协作律师" span={2}>
                    {item.co_lawyer_ids?.length ? item.co_lawyer_ids.map((id) => lawyerLabel(id)).join('、') : '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="对方当事人姓名">
                    {item.opponent_name || <Tag color="orange">资料缺失</Tag>}
                  </Descriptions.Item>
                  <Descriptions.Item label="对方证件号">
                    {item.opponent_id_number || <Tag color="orange">资料缺失</Tag>}
                  </Descriptions.Item>
                  <Descriptions.Item label="受理日期">{item.accept_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="结案日期">{item.close_date || '-'}</Descriptions.Item>
                  <Descriptions.Item label="摘要" span={2}>{item.summary || '-'}</Descriptions.Item>
                </Descriptions>
                <PermissionGuard roles={['admin', 'lawyer']}>
                  <Space style={{ marginTop: 16 }}>
                    <Select value={status} style={{ width: 150 }} options={CaseStatusOptions} onChange={setStatus} />
                    <Button type="primary" onClick={onStatusChange}>更新状态</Button>
                  </Space>
                  <Space style={{ marginTop: 8 }} align="start">
                    <Select
                      placeholder="分配主办律师"
                      style={{ width: 180 }}
                      value={lawyer}
                      onChange={(v) => { setLawyer(v); setConflict(null) }}
                      options={userStore.lawyers.map((l) => ({ label: l.real_name || l.username, value: l.id }))}
                    />
                    <Select
                      mode="multiple"
                      allowClear
                      placeholder="协作律师（可多选）"
                      style={{ minWidth: 240 }}
                      value={coLawyers}
                      onChange={(v) => { setCoLawyers(v); setConflict(null) }}
                      options={userStore.lawyers
                        .filter((l) => l.id !== lawyer)
                        .map((l) => ({ label: l.real_name || l.username, value: l.id }))}
                    />
                    <Button onClick={onAssign}>分配（利益冲突预检）</Button>
                  </Space>
                  {assignWarning && (
                    <Alert
                      style={{ marginTop: 12 }}
                      type="warning"
                      showIcon
                      message="已分配，但未完成利益冲突预检"
                      description={assignWarning}
                    />
                  )}
                  {conflict && (
                    <Alert
                      style={{ marginTop: 12 }}
                      type="error"
                      showIcon
                      action={<Button size="small" icon={<EditOutlined />} onClick={openEditOpponent}>补录/调整对方信息</Button>}
                      message="律师利益冲突预检未通过，本次分配已拒绝（原分配保持不变）"
                      description={
                        <div>
                          <div style={{ marginBottom: 8 }}>
                            冲突原因：{conflict.reason}。任一待分配律师正在未结案件中代理与本案对方证件号相同的当事人。
                          </div>
                          {conflict.conflicts.map((cf: LawyerConflictInfo) => (
                            <div key={cf.lawyer_id} style={{ marginBottom: 6 }}>
                              <Tag color="red">律师：{cf.lawyer_name}</Tag>
                              <span>对方：{cf.opponent_name}（证件号 {cf.opponent_id_number}）；</span>
                              <span>在未结案件中代理：{cf.client_name || '-'}；</span>
                              <span>冲突案号：
                                {cf.conflict_case_nos.map((no) => <Tag key={no} color="volcano">{no}</Tag>)}
                              </span>
                            </div>
                          ))}
                          <div style={{ color: '#888' }}>调整律师或补录对方信息后可点击「分配」重试。</div>
                        </div>
                      }
                    />
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
      <Space style={{ marginTop: 16 }}>
        <PermissionGuard roles={['admin', 'lawyer']}>
          <Button icon={<EditOutlined />} onClick={openEditOpponent}>编辑对方当事人信息</Button>
        </PermissionGuard>
      </Space>
      <Modal
        title="编辑对方当事人信息"
        open={editOpen}
        onOk={onEditOpponent}
        onCancel={() => setEditOpen(false)}
        destroyOnClose
      >
        <Form form={editForm} layout="vertical">
          <Form.Item name="opponent_name" label="对方当事人姓名" rules={[{ max: 100 }]}>
            <Input placeholder="缺失会导致分配时跳过冲突预检" />
          </Form.Item>
          <Form.Item name="opponent_id_number" label="对方证件号" rules={[{ max: 50 }]}>
            <Input placeholder="利益冲突预检以证件号为准" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}
