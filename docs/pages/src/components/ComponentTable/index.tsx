import React from 'react';

export type ComponentRow = {
  name: string;
  description: string;
  icon?: string;
  link?: {
    text?: string;
    url: string;
  };
};

type ComponentTableProps = {
  components: ComponentRow[];
};

/**
 * Table component for displaying component information
 */
export default function ComponentTable({
  components,
}: ComponentTableProps): JSX.Element {
  return (
    <div className="component-table">
      <table>
        <thead>
          <tr>
            <th>Component</th>
            <th>Description</th>
            <th>Link</th>
          </tr>
        </thead>
        <tbody>
          {components.map((component, idx) => (
            <tr key={idx}>
              <td>
                {component.icon && <span>{component.icon} </span>}
                <strong>{component.name}</strong>
              </td>
              <td>{component.description}</td>
              <td>
                {component.link && (
                  <a href={component.link.url}>
                    {component.link.text || 'Documentation →'}
                  </a>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}