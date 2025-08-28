import React, { ReactNode } from 'react';

type CardGridProps = {
  children: ReactNode;
  columns?: 1 | 2 | 3 | 4;
};

/**
 * Responsive card grid component with configurable columns
 */
export default function CardGrid({
  children,
  columns = 3,
}: CardGridProps): JSX.Element {
  // Calculate column width based on number of columns
  const colWidth = 12 / columns;
  const colClass = `col col--${colWidth}`;

  return (
    <div className="container">
      <div className="row">
        {React.Children.map(children, (child, index) => (
          <div key={index} className={colClass}>
            {child}
          </div>
        ))}
      </div>
    </div>
  );
}