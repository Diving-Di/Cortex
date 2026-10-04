import { useEffect, useRef } from 'react';
import { useBlocker } from 'react-router-dom';

export default function LeaveGuard({ dirty }: { dirty: boolean }) {
  const bypass = useRef(false);
  const blocker = useBlocker(() => dirty && !bypass.current);
  useEffect(() => {
    const handle = (event: Event) => {
      if (!dirty) return;
      if (!window.confirm('有未保存内容，退出后仍可在当前标签页恢复草稿。是否退出？'))
        event.preventDefault();
      else bypass.current = true;
    };
    window.addEventListener('cortex:before-navigate', handle);
    return () => window.removeEventListener('cortex:before-navigate', handle);
  }, [dirty]);
  useEffect(() => {
    bypass.current = false;
  }, [dirty]);
  useEffect(() => {
    if (blocker.state === 'blocked') {
      if (window.confirm('有未保存内容。离开后可在当前标签页恢复草稿，是否离开？'))
        blocker.proceed();
      else blocker.reset();
    }
  }, [blocker]);
  return null;
}
