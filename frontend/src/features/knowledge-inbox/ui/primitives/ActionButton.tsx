import { Icon } from "./Icon";

type ActionButtonProps = {
  label: string;
  isRunning: boolean;
  onClick: () => void;
};

export function ActionButton({ label, isRunning, onClick }: ActionButtonProps) {
  return (
    <button className="primary-action" type="button" onClick={onClick} disabled={isRunning}>
      <Icon name="run" />
      {label}
    </button>
  );
}
