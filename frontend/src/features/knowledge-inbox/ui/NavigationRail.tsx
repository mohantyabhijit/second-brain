import type { KnowledgeInboxViewModel } from "../presentation/viewModel";

type NavigationRailProps = {
  brand: KnowledgeInboxViewModel["brand"];
  navigation: KnowledgeInboxViewModel["navigation"];
  note: KnowledgeInboxViewModel["sidebarNote"];
};

export function NavigationRail({ brand, navigation, note }: NavigationRailProps) {
  return (
    <aside className="sidebar" aria-label="Product navigation">
      <div className="brand-lockup">
        <div className="brand-mark">{brand.mark}</div>
        <div>
          <strong>{brand.name}</strong>
          <span>{brand.descriptor}</span>
        </div>
      </div>
      <nav>
        {navigation.map((item) => (
          <a href={item.href} key={item.href}>
            {item.label}
          </a>
        ))}
      </nav>
      <div className="sidebar-note">
        <span>{note.label}</span>
        <strong>{note.value}</strong>
      </div>
    </aside>
  );
}
