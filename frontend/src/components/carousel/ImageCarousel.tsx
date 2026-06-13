import { Carousel, Empty, Image } from 'antd'
import type { ImageItem } from '../../types/image'
import { getFullImageUrl } from '../../utils/url'
import './ImageCarousel.css'

interface ImageCarouselProps {
  items?: ImageItem[]
}

export default function ImageCarousel({ items = [] }: ImageCarouselProps) {
  return (
    <div className="carousel-wrap">
      {!items || items.length === 0 ? (
        <Empty description="未选择轮播图片" />
      ) : (
        <Carousel dots arrows className="image-carousel">
          {items.map(item => (
            <div key={item.id}>
              <div className="carousel-slide">
                <Image
                  src={getFullImageUrl(item.image)}
                  className="carousel-image"
                  preview={{ src: getFullImageUrl(item.image) }}
                  fallback=""
                  alt={`图片 ${item.id}`}
                />
              </div>
            </div>
          ))}
        </Carousel>
      )}
    </div>
  )
}
