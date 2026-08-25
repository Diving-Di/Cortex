import io

import fitz
from docx import Document
from PIL import Image, ImageDraw, ImageFont

from app import parse_docx, parse_pdf, ocr_image


pdf = fitz.open()
page = pdf.new_page()
page.insert_text((72, 72), "Cortex PDF acceptance text")
assert "acceptance" in parse_pdf(pdf.tobytes())[0]["text"]

word = Document()
word.add_heading("Cortex Word", 1)
word.add_paragraph("DOCX acceptance text")
buffer = io.BytesIO()
word.save(buffer)
assert any("acceptance" in item["text"] for item in parse_docx(buffer.getvalue()))

image = Image.new("RGB", (1000, 240), "white")
font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", 72)
ImageDraw.Draw(image).text((40, 70), "Cortex OCR 2026", fill="black", font=font)
assert "2026" in ocr_image(image, image_index=1)["text"]

print("pdf=pass docx=pass image_ocr=pass")
