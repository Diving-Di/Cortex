export type ImageId = number | string

export interface TagRecord {
  id?: ImageId
  tag_name?: string
  tag_type?: string
}

export interface ImageItem {
  id: ImageId
  image: string
  thumbnail?: string
  tags?: TagRecord[]
  exif_datetime?: string
  created?: string
  location?: string
  resolution?: string
}
