import React from 'react';
import { render } from 'react-dom';

export function mount(element) {
  render(React.createElement('div'), element);
}
