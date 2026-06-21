import * as React from 'react'
import { SectionLabel } from '../../../dune-ui'
import type { CategorySectionProps } from './interfaces'
import { CategoryButton } from './CategoryButton'
import { CategoryPanel } from './CategoryPanel'

// CategorySection renders one labelled block (Advanced or Expert) of collapsible
// category cards. Empty sections render nothing.
export const CategorySection: React.FC<CategorySectionProps> = ({
  title, description, categories, expandedCategory, onToggle,
  searching, pending, onChange, onDelete, isAmpManaged,
}) => {
  if (categories.length === 0) return null

  // When searching, show all categories stacked vertically with their panels.
  if (searching) {
    return (
      <div>
        <SectionLabel>{title}</SectionLabel>
        <div className="text-xs text-muted mb-2">{description}</div>
        <div className="flex flex-col gap-2 mt-2">
          {categories.map(([cat, catItems]) => (
            <div key={cat}>
              <div className="mb-1">
                <CategoryButton cat={cat} catItems={catItems} isOpen onToggle={onToggle} />
              </div>
              <CategoryPanel
                cat={cat}
                catItems={catItems}
                searching
                pending={pending}
                onChange={onChange}
                onDelete={onDelete}
                onToggle={onToggle}
                isAmpManaged={isAmpManaged}
              />
            </div>
          ))}
        </div>
      </div>
    )
  }

  // Normal mode: expanded category renders above the grid of remaining buttons.
  const expandedEntry = categories.find(([cat]) => cat === expandedCategory)
  const gridEntries = categories.filter(([cat]) => cat !== expandedCategory)

  return (
    <div>
      <SectionLabel>{title}</SectionLabel>
      <div className="text-xs text-muted mb-2">{description}</div>

      {expandedEntry && (
        <div className="flex flex-col gap-1 mb-2">
          <CategoryButton
            cat={expandedEntry[0]}
            catItems={expandedEntry[1]}
            isOpen
            onToggle={onToggle}
          />
          <CategoryPanel
            cat={expandedEntry[0]}
            catItems={expandedEntry[1]}
            searching={false}
            pending={pending}
            onChange={onChange}
            onDelete={onDelete}
            onToggle={onToggle}
            isAmpManaged={isAmpManaged}
          />
        </div>
      )}

      {gridEntries.length > 0 && (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-2">
          {gridEntries.map(([cat, catItems]) => (
            <CategoryButton
              key={cat}
              cat={cat}
              catItems={catItems}
              isOpen={false}
              onToggle={onToggle}
            />
          ))}
        </div>
      )}
    </div>
  )
}
