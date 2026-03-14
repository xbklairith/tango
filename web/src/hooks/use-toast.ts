import { useState, useCallback, useEffect } from "react";

interface Toast {
  id: string;
  title: string;
  description?: string;
  variant?: "default" | "destructive";
}

interface ToastInput {
  title: string;
  description?: string;
  variant?: "default" | "destructive";
}

let toastCount = 0;

const listeners = new Set<(toasts: Toast[]) => void>();
let memoryToasts: Toast[] = [];

function dispatch(toasts: Toast[]) {
  memoryToasts = toasts;
  listeners.forEach((l) => l(toasts));
}

export function useToast() {
  const [toasts, setToasts] = useState<Toast[]>(memoryToasts);

  useEffect(() => {
    listeners.add(setToasts);
    return () => {
      listeners.delete(setToasts);
    };
  }, []);

  const toast = useCallback((input: ToastInput) => {
    const id = String(++toastCount);
    const newToast: Toast = { ...input, id };
    dispatch([...memoryToasts, newToast]);
    setTimeout(() => {
      dispatch(memoryToasts.filter((t) => t.id !== id));
    }, 5000);
  }, []);

  const dismiss = useCallback((id: string) => {
    dispatch(memoryToasts.filter((t) => t.id !== id));
  }, []);

  return { toasts, toast, dismiss };
}
