import * as React from 'react'
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
  Select,
  SelectItem,
  toast,
} from '@heroui/react'
import { api } from '../api/client'

export interface CreateGuildModalProps {
  isOpen: boolean
  onClose: () => void
  onSuccess: (guildId: number) => void
}

export const CreateGuildModal: React.FC<CreateGuildModalProps> = ({ isOpen, onClose, onSuccess }) => {
  const [name, setName] = React.useState('')
  const [desc, setDesc] = React.useState('')
  const [faction, setFaction] = React.useState<number>(1)
  const [loading, setLoading] = React.useState(false)

  const handleSubmit = async () => {
    if (!name.trim()) {
      toast.danger('Name cannot be empty')
      return
    }
    setLoading(true)
    try {
      const res = await api.guilds.create({ name, description: desc, faction_id: faction })
      toast.success('Guild created')
      onSuccess(res.guild_id)
      onClose()
    } catch (e: any) {
      toast.danger(e.message || 'Failed to create guild')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal isOpen={isOpen} onClose={onClose}>
      <ModalContent>
        <ModalHeader>Create NPC Guild</ModalHeader>
        <ModalBody>
          <Input 
            label="Guild Name" 
            value={name} 
            onChange={e => setName(e.target.value)} 
          />
          <Input 
            label="Description" 
            value={desc} 
            onChange={e => setDesc(e.target.value)} 
          />
          <Select 
            label="Faction" 
            selectedKeys={new Set([faction.toString()])} 
            onSelectionChange={(keys) => {
              const k = Array.from(keys)[0] as string
              if (k) setFaction(parseInt(k, 10))
            }}
          >
            <SelectItem key="1" textValue="Atreides">Atreides</SelectItem>
            <SelectItem key="2" textValue="Harkonnen">Harkonnen</SelectItem>
          </Select>
        </ModalBody>
        <ModalFooter>
          <Button variant="flat" onPress={onClose}>Cancel</Button>
          <Button color="primary" isLoading={loading} onPress={handleSubmit}>Create</Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}
