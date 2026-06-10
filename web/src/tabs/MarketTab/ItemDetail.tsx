import type React from 'react'
import { useEffect, useState } from 'react'
import { Drawer, Spinner } from '@heroui/react'
import { useTranslation } from 'react-i18next'
import { useAtomValue } from 'jotai'
import { unwrap } from 'jotai/utils'
import { api } from '../../api/client'
import type { MarketItem, MarketListing } from '../../api/client'
import { Panel, SectionLabel } from '../../dune-ui'
import { iconUrl, qualityLabel } from '../../utils/icons'
import { getItemEntry } from '../../data/itemData'
import { qualityDataAtom } from '../../data/store'

const qualityDataSync = unwrap(qualityDataAtom, () => null)

type ItemEntry = {
  is_gradeable?: boolean
  armor_value?: number
  mitigation?: Record<string, number>
}

const QUALITY_LABELS = ['Standard', 'Refined', 'Superior', 'Masterwork', 'Pristine', 'Flawless']

const MITIGATION_LABELS: Record<string, string> = {
  melee: 'Melee',
  physical: 'Physical',
  energy: 'Energy',
  explosive: 'Explosive',
  heat: 'Heat',
  cold: 'Cold',
  poison: 'Poison',
  radiation: 'Radiation',
  sandstorm1: 'Sandstorm I',
  sandstorm2: 'Sandstorm II',
  sandstorm3: 'Sandstorm III',
}

interface ItemDetailProps {
  item: MarketItem | null
  onClose: () => void
}

interface RowProps {
  label: string
  value: string
  accent?: boolean
  wrap?: boolean
}

export const ItemDetail: React.FC<ItemDetailProps> = ({ item, onClose }) => {
  const { t } = useTranslation()
  const [listings, setListings] = useState<MarketListing[]>([])
  const [loading, setLoading] = useState(false)
  const [entry, setEntry] = useState<ItemEntry | null>(null)
  const qualityData = useAtomValue(qualityDataSync)

  useEffect(() => {
    if (!item) return
    Promise.resolve()
      .then(() => {
        setListings([])
        setEntry(null)
        setLoading(true)
      })
      .then(() => Promise.all([
        api.market.listings(item.template_id),
        getItemEntry(item.template_id),
      ]))
      .then(([ls, e]) => {
        setListings(ls)
        setEntry(e)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [item])

  const img = item ? iconUrl(item.template_id, 'thumb') : null
  const byQuality = listings.reduce<Record<number, MarketListing[]>>((acc, l) => {
    ;(acc[l.quality] ??= []).push(l)
    return acc
  }, {})
  const qualities = Object.keys(byQuality).map(Number).sort((a, b) => a - b)

  const isArmor = !!entry?.armor_value
  const isWeapon = item?.category?.startsWith('items/weapons')
  const isGradeable = entry?.is_gradeable

  return (
    <Drawer.Backdrop variant="opaque" isOpen={!!item} onOpenChange={(v) => !v && onClose()}>
      <Drawer.Content placement="right">
        <Drawer.Dialog className="w-[480px] max-w-[95vw] flex flex-col">
          <Drawer.Header>
            <div className="flex items-center gap-2 px-4 py-3 border-b border-border w-full">
              {img && (
                <img
                  src={img}
                  alt=""
                  className="w-7 h-7 object-contain shrink-0 rounded"
                  onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none' }}
                />
              )}
              <Drawer.Heading className="font-semibold text-sm text-accent truncate flex-1">
                {item?.display_name || item?.template_id || ''}
              </Drawer.Heading>
              <Drawer.CloseTrigger />
            </div>
          </Drawer.Header>

          <Drawer.Body className="flex flex-col gap-3 p-3">
            {item && (
              <>
                <Panel>
                  <SectionLabel>{t('market.itemDetail.itemInfo')}</SectionLabel>
                  <Row label={t('market.itemDetail.category')} value={item.category || '—'} wrap />
                  <Row label={t('market.itemDetail.tierLabel')} value={item.tier > 0 ? String(item.tier) : '—'} />
                  <Row label={t('market.itemDetail.rarityLabel')} value={item.rarity || '—'} />
                  <Row label={t('market.itemDetail.totalStock')} value={item.total_stock.toLocaleString()} />
                  <Row label={t('market.itemDetail.botStock')} value={item.bot_stock.toLocaleString()} />
                  <Row label={t('market.itemDetail.listingsLabel')} value={String(item.listing_count)} />
                  <Row label={t('market.itemDetail.lowestPrice')} value={item.lowest_price.toLocaleString()} accent />
                </Panel>

                {isArmor && (
                  <Panel>
                    <SectionLabel>{t('market.itemDetail.armorStats')}</SectionLabel>
                    {isGradeable
                      ? (
                          <>
                            <div className="text-xs text-muted mb-1">{t('market.itemDetail.armorValueByQuality')}</div>
                            <table className="w-full text-xs mb-2">
                              <thead>
                                <tr className="text-muted">
                                  {QUALITY_LABELS.map((ql, i) => (
                                    <th key={i} className="text-center pb-1 font-normal">{ql.slice(0, 3)}</th>
                                  ))}
                                </tr>
                              </thead>
                              <tbody>
                                <tr>
                                  {(qualityData?.armor ?? []).map((mult, i) => (
                                    <td key={i} className="text-center font-mono text-foreground">
                                      {Math.round(entry!.armor_value! * mult)}
                                    </td>
                                  ))}
                                </tr>
                              </tbody>
                            </table>
                          </>
                        )
                      : (
                          <Row label={t('market.itemDetail.armorValue')} value={String(entry!.armor_value)} />
                        )}
                    {entry?.mitigation && Object.keys(entry.mitigation).length > 0 && (
                      <>
                        <div className="text-xs text-muted mt-1 mb-1">{t('market.itemDetail.resistances')}</div>
                        {Object.entries(entry.mitigation).map(([k, v]) => (
                          <Row
                            key={k}
                            label={MITIGATION_LABELS[k] ?? k}
                            value={`${Math.round(v * 100)}%`}
                          />
                        ))}
                      </>
                    )}
                  </Panel>
                )}

                {isWeapon && isGradeable && (
                  <Panel>
                    <SectionLabel>{t('market.itemDetail.weaponQualityScaling')}</SectionLabel>
                    <div className="text-xs text-muted mb-1">{t('market.itemDetail.damageMultiplierByQuality')}</div>
                    <table className="w-full text-xs">
                      <thead>
                        <tr className="text-muted">
                          {QUALITY_LABELS.map((ql, i) => (
                            <th key={i} className="text-center pb-1 font-normal">{ql.slice(0, 3)}</th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        <tr>
                          {(qualityData?.weapon_damage ?? []).map((mult, i) => (
                            <td key={i} className="text-center font-mono text-foreground">
                              {mult.toFixed(2)}
                              ×
                            </td>
                          ))}
                        </tr>
                      </tbody>
                    </table>
                  </Panel>
                )}

                <Panel>
                  <SectionLabel>{t('market.itemDetail.activeListings')}</SectionLabel>
                  {loading
                    ? (
                        <div className="flex justify-center py-4"><Spinner size="sm" /></div>
                      )
                    : listings.length === 0
                      ? (
                          <p className="text-xs text-muted">{t('market.itemDetail.noActiveListings')}</p>
                        )
                      : (
                          <div className="flex flex-col gap-3">
                            {qualities.map((q) => (
                              <div key={q}>
                                <div className="text-xs font-medium text-muted mb-1">{qualityLabel(q)}</div>
                                <table className="w-full text-xs">
                                  <thead>
                                    <tr className="text-muted">
                                      <th className="text-left pb-1 font-normal">{t('market.itemDetail.seller')}</th>
                                      <th className="text-right pb-1 font-normal">{t('market.itemDetail.stockCol')}</th>
                                      <th className="text-right pb-1 font-normal">{t('market.itemDetail.priceCol')}</th>
                                    </tr>
                                  </thead>
                                  <tbody>
                                    {byQuality[q].sort((a, b) => a.price - b.price).map((l) => (
                                      <tr key={l.order_id} className="border-t border-border/40">
                                        <td className={`py-0.5 ${l.owner_type === 'bot' ? 'text-accent' : 'text-foreground'}`}>
                                          {l.owner_name}
                                        </td>
                                        <td className="py-0.5 text-right text-muted">{l.stock.toLocaleString()}</td>
                                        <td className="py-0.5 text-right font-mono">{l.price.toLocaleString()}</td>
                                      </tr>
                                    ))}
                                  </tbody>
                                </table>
                              </div>
                            ))}
                          </div>
                        )}
                </Panel>
              </>
            )}
          </Drawer.Body>
        </Drawer.Dialog>
      </Drawer.Content>
    </Drawer.Backdrop>
  )
}

function Row({ label, value, accent, wrap }: RowProps) {
  return (
    <div className={`flex text-xs py-0.5 ${wrap ? 'flex-col gap-0.5' : 'items-center justify-between'}`}>
      <span className="text-muted shrink-0">{label}</span>
      <span className={accent ? 'font-mono text-accent font-semibold' : 'text-foreground'}>{value}</span>
    </div>
  )
}
