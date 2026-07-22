type Props = { title: string };

export default function PlaceholderPage({ title }: Props) {
  return (
    <section>
      <h1>{title}</h1>
      <p>该功能将在对应 SDD 迭代中实现。</p>
    </section>
  );
}
