export const KEG_STATUS = {
  in_warehouse: '在库',
  in_transit: '配送中',
  at_bar: '在店未开桶',
  opened: '已开桶',
  empty: '已打空',
  pickup_booked: '已预约回收',
  in_return: '回收在途',
  at_wash: '待清洗',
  ready_refill: '待再灌装',
  wash_failed: '清洗不合格',
  lost: '已丢失',
}
export const SHIPMENT_STATUS = {
  pending: '待配送',
  in_transit: '配送中',
  delivered: '已送达',
  received: '已签收',
  held: '已暂缓',
  cancelled: '已取消',
}
export const PICKUP_STATUS = { booked: '已预约', completed: '已完成' }
export const CASE_TYPE = {
  late_return: '延迟归还',
  lost: '酒桶丢失',
  damaged: '桶体变形/破损',
  taste_complaint: '批次口味投诉',
  missed_scan: '司机漏扫',
  wash_failed: '清洗不合格',
}
export const CASE_STATUS = { open: '待处理', processing: '处理中', resolved: '已结案' }
export const BATCH_STATUS = { active: '在售', suspended: '已停售', investigating: '排查暂停配送', recalled: '已召回' }
export const INV_STATUS = { investigating: '排查中', concluded: '已出具结论' }
export const INV_CONCLUSION = { recall: '确认召回', discount: '已开桶折价', clear: '排除批次问题' }
export const RESTOCK_STATUS = { open: '待确认批次', stocking: '备货中', done: '备货完成' }
export const ENTRY_TYPE = { charge: '收取', release: '释放', deduction: '扣减', discount: '折价补偿' }
export const DEPOSIT_STATUS = { pending: '待结算', settled: '已结算' }
export const ROLE = {
  admin: '管理员',
  warehouse: '酒厂仓库',
  bar_manager: '酒吧店长',
  driver: '配送司机',
  washer: '清洗厂',
  qc: '品控',
  finance: '财务',
  dealer: '经销商',
  sales: '销售',
}
export const TONE = {
  in_warehouse: 'gray', in_transit: 'blue', at_bar: 'green', opened: 'amber',
  empty: 'gray', pickup_booked: 'blue', in_return: 'blue', at_wash: 'purple',
  ready_refill: 'green', wash_failed: 'red', lost: 'red',
  active: 'green', suspended: 'amber', investigating: 'red', recalled: 'red',
  open: 'red', processing: 'amber', resolved: 'green', concluded: 'green',
  pending: 'amber', settled: 'green', booked: 'blue', completed: 'green', stocking: 'amber', done: 'green',
  delivered: 'blue', received: 'green', held: 'red', cancelled: 'gray',
  charge: 'blue', release: 'green', deduction: 'red', discount: 'amber',
  pass: 'green', fail: 'red',
}
export const APPEARANCES = ['完好', '变形', '破损']
export const RESIDUALS = ['无', '少量', '较多']
export const CAP_STATUSES = ['完好', '缺失', '损坏']
export const POUR_SPEEDS = ['正常', '偏慢', '偏快']
export const EQUIPMENTS = ['板式扎啤机', '直连酒柱/酒头', '便携式出酒器', '其他（填详情）']
export const TASTE_DESCS = ['氧化味（纸板/陈旧）', '哈喇味', '酸败/染菌', '金属味', '异味但说不清']
