import React from 'react';

type TocItem = {
  title: string;
  links: Array<{
    text: string;
    url: string;
  }>;
  icon?: string;
};

type TableOfContentsProps = {
  items: TocItem[];
};

/**
 * Table of contents component for navigation within a page
 */
export default function TableOfContents({
  items,
}: TableOfContentsProps): JSX.Element {
  return (
    <div className="container">
      <div className="row">
        {items.map((item, idx) => (
          <div key={idx} className="col col--6">
            <div className="card margin-bottom--lg">
              <div className="card__body">
                <h3>{item.icon && <span>{item.icon} </span>}{item.title}</h3>
                <ul>
                  {item.links.map((link, linkIdx) => (
                    <li key={linkIdx}>
                      <a href={link.url}>{link.text}</a>
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}