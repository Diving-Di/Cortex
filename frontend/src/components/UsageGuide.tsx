import { Button, Modal, Steps } from 'antd';
import { useState } from 'react';
import { useDraftScope } from '../app/drafts';

export default function UsageGuide({
  id,
  steps,
}: {
  id: string;
  steps: { title: string; description: string }[];
}) {
  const scope = useDraftScope();
  const key = `cortex:guide:${encodeURIComponent(scope)}:${id}`;
  const [open, setOpen] = useState(() => {
    try {
      return !localStorage.getItem(key);
    } catch {
      return false;
    }
  });
  const [current, setCurrent] = useState(0);
  function close() {
    setOpen(false);
    try {
      localStorage.setItem(key, 'seen');
    } catch {
      /* Optional preference. */
    }
  }
  return (
    <>
      <Button
        size="small"
        onClick={() => {
          setCurrent(0);
          setOpen(true);
        }}
      >
        使用引导
      </Button>
      <Modal
        title="开始使用"
        open={open}
        onCancel={close}
        footer={[
          <Button key="skip" onClick={close}>
            跳过
          </Button>,
          <Button
            key="next"
            type="primary"
            onClick={() => (current === steps.length - 1 ? close() : setCurrent(current + 1))}
          >
            {current === steps.length - 1 ? '完成' : '下一步'}
          </Button>,
        ]}
      >
        <Steps direction="vertical" current={current} items={steps} />
      </Modal>
    </>
  );
}
